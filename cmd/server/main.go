// @title           Sentinel API
// @version         1.0
// @description     Servicio centralizado de autenticación y autorización con JWT RS256, RBAC y auditoría.
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Token JWT de acceso. Formato: "Bearer {token}"
// @securityDefinitions.apikey AppKeyAuth
// @in              header
// @name            X-App-Key
// @description     Clave secreta de la aplicación cliente

// Package main es el punto de entrada del servicio Sentinel.
// Realiza en orden las siguientes tareas al iniciar:
//  1. Carga la configuración desde config.yaml (o la ruta indicada por CONFIG_PATH).
//  2. Inicializa el logger estructurado (slog) y lo establece como logger global.
//  3. Crea el pool de conexiones a PostgreSQL (pgxpool) y verifica conectividad.
//  4. Crea el cliente Redis y verifica conectividad.
//  5. Carga las claves RSA para firma/verificación JWT RS256.
//  6. Instancia todos los repositorios (postgres + redis).
//  7. Instancia todos los servicios de negocio.
//  8. Ejecuta el bootstrap del sistema (solo si la tabla applications está vacía).
//  9. Instancia los handlers HTTP.
// 10. Configura la aplicación Fiber con middlewares globales y rutas.
// 11. Arranca el servidor HTTP y espera señales de apagado (SIGINT/SIGTERM).
// 12. Realiza un apagado ordenado (graceful shutdown) con timeout configurable.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	swagger "github.com/gofiber/swagger"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	swaggerdocs "github.com/enunezf/sentinel/docs/api"
	"github.com/enunezf/sentinel/internal/bootstrap"
	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/handler"
	"github.com/enunezf/sentinel/internal/logger"
	"github.com/enunezf/sentinel/internal/middleware"
	pgrepository "github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepository "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/service"
	"github.com/enunezf/sentinel/internal/token"
)

// main es la función principal del servidor Sentinel.
// Orquesta la inicialización de todas las dependencias y el ciclo de vida del servidor HTTP.
// Si alguna dependencia crítica falla (base de datos, Redis, claves RSA, bootstrap),
// el proceso termina inmediatamente con os.Exit(1) para evitar arrancar en un estado inválido.
func main() {
	// Determinar la ruta del archivo de configuración.
	// Por defecto se usa "config.yaml" en el directorio de trabajo actual.
	// La variable de entorno CONFIG_PATH permite sobrescribir esta ruta
	// (útil en contenedores donde la configuración se monta en una ruta específica).
	configPath := "config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		// El logger aún no está disponible; usamos el logger por defecto de slog.
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	// Crear el logger estructurado como primera dependencia del sistema.
	// Se establece también como logger global de slog para que las bibliotecas
	// que usen slog.Default() también escriban en el mismo destino configurado.
	appLogger := logger.New(cfg.Logging)
	slog.SetDefault(appLogger)

	// -----------------------------------------------------------------------
	// Pool de conexiones PostgreSQL
	// -----------------------------------------------------------------------

	// Parsear la cadena de conexión DSN al formato de configuración de pgxpool.
	pgCfg, err := pgxpool.ParseConfig(cfg.Database.DSN())
	if err != nil {
		appLogger.Error("cannot parse database DSN", "error", err, "component", "database")
		os.Exit(1)
	}

	// Configurar los límites del pool de conexiones a partir de la configuración.
	// MaxConns limita el total de conexiones simultáneas para no sobrecargar PostgreSQL.
	// MinConns mantiene conexiones abiertas para reducir latencia en picos de carga.
	pgCfg.MaxConns = int32(cfg.Database.MaxOpenConns)
	pgCfg.MinConns = int32(cfg.Database.MaxIdleConns)
	pgCfg.MaxConnLifetime = cfg.Database.ConnMaxLifetime

	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		appLogger.Error("cannot create PostgreSQL pool", "error", err, "component", "database")
		os.Exit(1)
	}
	// Cerrar el pool al terminar para liberar conexiones de forma limpia.
	defer pool.Close()

	// Verificar conectividad real con la base de datos antes de continuar.
	if err := pool.Ping(ctx); err != nil {
		appLogger.Error("cannot connect to PostgreSQL", "error", err, "component", "database")
		os.Exit(1)
	}
	appLogger.Info("PostgreSQL connection pool established", "component", "database")

	// -----------------------------------------------------------------------
	// Conexión Redis
	// -----------------------------------------------------------------------

	// Redis se usa para dos propósitos:
	// 1. Almacenar hashes de refresh tokens con TTL automático.
	// 2. Cachear el mapa de permisos por usuario para reducir consultas a PostgreSQL.
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			appLogger.Warn("error closing Redis client", "error", err, "component", "redis")
		}
	}()

	// Verificar conectividad con Redis antes de continuar.
	if err := rdb.Ping(ctx).Err(); err != nil {
		appLogger.Error("cannot connect to Redis", "error", err, "component", "redis")
		os.Exit(1)
	}
	appLogger.Info("Redis connection established", "component", "redis")

	// -----------------------------------------------------------------------
	// Gestor de tokens JWT (claves RSA)
	// -----------------------------------------------------------------------

	// Cargar el par de claves RSA desde los archivos PEM configurados.
	// La clave privada se usa para firmar; la clave pública se usa para verificar
	// y se expone en /.well-known/jwks.json para los backends consumidores.
	tokenMgr, err := token.NewManager(cfg.JWT.PrivateKeyPath, cfg.JWT.PublicKeyPath)
	if err != nil {
		appLogger.Error("cannot load RSA keys", "error", err, "component", "token")
		os.Exit(1)
	}
	appLogger.Info("RSA key pair loaded", "component", "token")

	// -----------------------------------------------------------------------
	// Repositorios
	// -----------------------------------------------------------------------

	// Cada repositorio encapsula el acceso a datos de una entidad específica.
	// Los repositorios de PostgreSQL usan pgxpool para operaciones persistentes.
	// Los repositorios de Redis manejan datos efímeros (tokens, caché de permisos).
	userRepo := pgrepository.NewUserRepository(pool, appLogger)
	appRepo := pgrepository.NewApplicationRepository(pool, appLogger)
	refreshPGRepo := pgrepository.NewRefreshTokenRepository(pool, appLogger)
	auditRepo := pgrepository.NewAuditRepository(pool, appLogger)
	roleRepo := pgrepository.NewRoleRepository(pool, appLogger)
	permRepo := pgrepository.NewPermissionRepository(pool, appLogger)
	ccRepo := pgrepository.NewCostCenterRepository(pool, appLogger)
	userRoleRepo := pgrepository.NewUserRoleRepository(pool, appLogger)
	userPermRepo := pgrepository.NewUserPermissionRepository(pool, appLogger)
	userCCRepo := pgrepository.NewUserCostCenterRepository(pool, appLogger)
	pwdHistRepo := pgrepository.NewPasswordHistoryRepository(pool, appLogger)

	// Repositorio de refresh tokens en Redis (almacena el hash con TTL automático).
	refreshRedisRepo := redisrepository.NewRefreshTokenRepository(rdb, appLogger)
	// Caché de permisos en Redis (reduce consultas a PostgreSQL en authz frecuente).
	authzCache := redisrepository.NewAuthzCache(rdb, appLogger)

	// -----------------------------------------------------------------------
	// Servicios de negocio
	// -----------------------------------------------------------------------

	// auditSvc es el servicio de auditoría asíncrona. Recibe eventos a través de un canal
	// con buffer de tamaño 1000 y los persiste en PostgreSQL sin bloquear el flujo HTTP.
	// Se llama Close() al apagar para vaciar el canal pendiente antes de terminar.
	auditSvc := service.NewAuditService(auditRepo, appLogger)
	defer auditSvc.Close()

	// authSvc maneja login, refresh, logout y cambio de contraseña.
	authSvc := service.NewAuthService(
		userRepo, appRepo, refreshPGRepo, refreshRedisRepo,
		pwdHistRepo, userRoleRepo, tokenMgr, auditSvc, cfg,
	)

	// authzSvc maneja verificación de permisos, mapa de permisos y caché.
	authzSvc := service.NewAuthzService(
		appRepo, userRoleRepo, userPermRepo, userCCRepo,
		permRepo, roleRepo, ccRepo, authzCache, tokenMgr, auditSvc,
	)

	// userSvc maneja la gestión de usuarios: CRUD, asignaciones y desbloqueo.
	userSvc := service.NewUserService(
		userRepo, userRoleRepo, userPermRepo, userCCRepo,
		refreshPGRepo, pwdHistRepo, appRepo, auditSvc, cfg,
	)

	// Servicios de administración para roles, permisos y centros de costo.
	roleSvc := service.NewRoleService(roleRepo, permRepo, appRepo, authzCache, auditSvc)
	permSvc := service.NewPermissionService(permRepo, appRepo, authzCache, auditSvc)
	ccSvc := service.NewCostCenterService(ccRepo, appRepo, authzCache, auditSvc)

	// -----------------------------------------------------------------------
	// Bootstrap del sistema
	// -----------------------------------------------------------------------

	// El bootstrap se ejecuta una única vez cuando la tabla applications está vacía.
	// Crea la aplicación raíz "sentinel", el usuario administrador inicial con los
	// permisos de sistema, y los roles predeterminados. Si ya existe al menos una
	// aplicación, el bootstrap no hace nada (idempotente).
	initializer := bootstrap.NewInitializer(appRepo, userRepo, roleRepo, permRepo, userRoleRepo, auditRepo, cfg, appLogger)
	if err := initializer.Initialize(ctx); err != nil {
		appLogger.Error("bootstrap failed", "error", err, "component", "bootstrap")
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Handlers HTTP
	// -----------------------------------------------------------------------

	// Cada handler agrupa los endpoints de un dominio y delega la lógica a los servicios.
	authHandler := handler.NewAuthHandler(authSvc, tokenMgr, appLogger)
	authzHandler := handler.NewAuthzHandler(authzSvc, appLogger)
	adminHandler := handler.NewAdminHandler(userSvc, roleSvc, permSvc, ccSvc, auditRepo, appRepo, appLogger)

	// -----------------------------------------------------------------------
	// Aplicación Fiber
	// -----------------------------------------------------------------------

	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		// ErrorHandler centralizado: registra errores 5xx en el logger y retorna
		// un JSON uniforme con la estructura {"error": {"code": ..., "message": ...}}.
		// Este handler captura tanto panics (vía recover) como errores retornados
		// explícitamente por los handlers.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			requestID, _ := c.Locals("request_id").(string)
			code := fiber.StatusInternalServerError
			msg := "internal server error"
			// Si es un *fiber.Error, usa su código y mensaje específicos.
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			}
			// Solo registrar como error los códigos 5xx; los 4xx son errores del cliente.
			if code >= 500 {
				appLogger.Error("unhandled server error",
					"error", err,
					"status", code,
					"path", c.Path(),
					"method", c.Method(),
					"request_id", requestID,
				)
			}
			return c.Status(code).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "INTERNAL_ERROR",
					"message": msg,
					"details": nil,
				},
			})
		},
	})

	// Limpiar el host hardcodeado del spec de Swagger para que la UI use el host
	// real de la solicitud HTTP en lugar del valor de compilación "localhost:8080".
	swaggerdocs.SwaggerInfo.Host = ""

	// -----------------------------------------------------------------------
	// Middlewares globales (se aplican a todas las rutas en el orden declarado)
	// -----------------------------------------------------------------------

	// recover: captura panics en handlers y los convierte en respuesta 500,
	// evitando que el proceso del servidor termine abruptamente.
	app.Use(recover.New())

	// cors: habilita CORS global para permitir que el dashboard React (puerto 8090)
	// y Swagger UI puedan hacer solicitudes directas a la API.
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-App-Key",
	}))

	// RequestID: genera un ID único por solicitud y lo almacena en c.Locals("request_id").
	// Se incluye en los logs para correlacionar todos los mensajes de una misma petición.
	app.Use(middleware.RequestID())

	// RequestLogger: registra cada petición HTTP con método, ruta, status y latencia.
	app.Use(middleware.RequestLogger(appLogger))

	// SecurityHeaders: agrega cabeceras de seguridad HTTP a todas las respuestas:
	// Strict-Transport-Security, X-Content-Type-Options, X-Frame-Options.
	app.Use(middleware.SecurityHeaders())

	// AuditContext: extrae IP y User-Agent de la solicitud y los almacena en c.Locals
	// para que los servicios de auditoría los incluyan sin recibir el contexto HTTP.
	app.Use(middleware.AuditContext())

	// -----------------------------------------------------------------------
	// Atajos de middleware para rutas protegidas
	// -----------------------------------------------------------------------

	// appKeyMW valida el header X-App-Key contra la tabla applications.
	appKeyMW := middleware.AppKey(appRepo, appLogger)

	// jwtMW valida el Bearer Token JWT RS256 en el header Authorization.
	jwtMW := middleware.JWTAuth(tokenMgr, appLogger)

	// requirePerm genera un middleware que verifica si el usuario autenticado
	// posee un permiso específico. Se usa en cada endpoint de administración.
	requirePerm := func(code string) fiber.Handler {
		return middleware.RequirePermission(authzSvc, code, appLogger)
	}

	// -----------------------------------------------------------------------
	// Definición de rutas
	// -----------------------------------------------------------------------

	// Swagger UI: accesible sin autenticación para facilitar el desarrollo y las pruebas.
	app.Get("/swagger/*", swagger.HandlerDefault)

	// Endpoints públicos: no requieren X-App-Key ni JWT.
	app.Get("/health", healthHandler(pool, rdb))
	// JWKS: expone la clave pública RSA en formato JWK para que los backends
	// consumidores puedan validar tokens RS256 localmente sin contactar a Sentinel.
	app.Get("/.well-known/jwks.json", authHandler.JWKS)

	// Endpoints de autenticación: requieren X-App-Key válido.
	auth := app.Group("/auth", appKeyMW)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	// logout y change-password además requieren JWT válido.
	auth.Post("/logout", jwtMW, authHandler.Logout)
	auth.Post("/change-password", jwtMW, authHandler.ChangePassword)

	// Endpoints de autorización.
	authz := app.Group("/authz")
	// verify: verifica si el usuario autenticado tiene un permiso específico.
	authz.Post("/verify", appKeyMW, jwtMW, authzHandler.Verify)
	// me/permissions: retorna todos los permisos del usuario autenticado (solo JWT).
	authz.Get("/me/permissions", jwtMW, authzHandler.MePermissions)
	// permissions-map: retorna el mapa completo de permisos por usuario (solo app_key).
	authz.Get("/permissions-map", appKeyMW, authzHandler.PermissionsMap)
	authz.Get("/permissions-map/version", appKeyMW, authzHandler.PermissionsMapVersion)

	// Endpoints de administración: requieren X-App-Key + JWT válido + permiso específico.
	admin := app.Group("/admin", appKeyMW, jwtMW)

	// Gestión de usuarios.
	admin.Get("/users", requirePerm("admin.users.read"), adminHandler.ListUsers)
	admin.Post("/users", requirePerm("admin.users.write"), adminHandler.CreateUser)
	admin.Get("/users/:id", requirePerm("admin.users.read"), adminHandler.GetUser)
	admin.Put("/users/:id", requirePerm("admin.users.write"), adminHandler.UpdateUser)
	admin.Post("/users/:id/unlock", requirePerm("admin.users.write"), adminHandler.UnlockUser)
	admin.Post("/users/:id/reset-password", requirePerm("admin.users.write"), adminHandler.ResetPassword)
	admin.Post("/users/:id/roles", requirePerm("admin.roles.write"), adminHandler.AssignRole)
	admin.Delete("/users/:id/roles/:rid", requirePerm("admin.roles.write"), adminHandler.RevokeRole)
	admin.Post("/users/:id/permissions", requirePerm("admin.permissions.write"), adminHandler.AssignPermission)
	admin.Delete("/users/:id/permissions/:pid", requirePerm("admin.permissions.write"), adminHandler.RevokePermission)
	admin.Post("/users/:id/cost-centers", requirePerm("admin.cost_centers.write"), adminHandler.AssignCostCenters)

	// Gestión de roles.
	admin.Get("/roles", requirePerm("admin.roles.read"), adminHandler.ListRoles)
	admin.Post("/roles", requirePerm("admin.roles.write"), adminHandler.CreateRole)
	admin.Get("/roles/:id", requirePerm("admin.roles.read"), adminHandler.GetRole)
	admin.Put("/roles/:id", requirePerm("admin.roles.write"), adminHandler.UpdateRole)
	admin.Delete("/roles/:id", requirePerm("admin.roles.write"), adminHandler.DeleteRole)
	admin.Post("/roles/:id/permissions", requirePerm("admin.permissions.write"), adminHandler.AddRolePermission)
	admin.Delete("/roles/:id/permissions/:pid", requirePerm("admin.permissions.write"), adminHandler.RemoveRolePermission)

	// Gestión de permisos.
	admin.Get("/permissions", requirePerm("admin.permissions.read"), adminHandler.ListPermissions)
	admin.Post("/permissions", requirePerm("admin.permissions.write"), adminHandler.CreatePermission)
	admin.Delete("/permissions/:id", requirePerm("admin.permissions.write"), adminHandler.DeletePermission)

	// Gestión de centros de costo.
	admin.Get("/cost-centers", requirePerm("admin.cost_centers.read"), adminHandler.ListCostCenters)
	admin.Post("/cost-centers", requirePerm("admin.cost_centers.write"), adminHandler.CreateCostCenter)
	admin.Put("/cost-centers/:id", requirePerm("admin.cost_centers.write"), adminHandler.UpdateCostCenter)

	// Gestión de aplicaciones registradas.
	admin.Get("/applications", requirePerm("admin.system.manage"), adminHandler.ListApplications)
	admin.Post("/applications", requirePerm("admin.system.manage"), adminHandler.CreateApplication)
	admin.Get("/applications/:id", requirePerm("admin.system.manage"), adminHandler.GetApplication)
	admin.Put("/applications/:id", requirePerm("admin.system.manage"), adminHandler.UpdateApplication)
	admin.Post("/applications/:id/rotate-key", requirePerm("admin.system.manage"), adminHandler.RotateApplicationKey)

	// Consulta de logs de auditoría.
	admin.Get("/audit-logs", requirePerm("admin.audit.read"), adminHandler.ListAuditLogs)

	// -----------------------------------------------------------------------
	// Arranque del servidor y apagado ordenado (graceful shutdown)
	// -----------------------------------------------------------------------

	// Canal para recibir señales del sistema operativo (Ctrl+C o SIGTERM de Kubernetes/Docker).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Canal para capturar errores de arranque del servidor HTTP.
	serverErr := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		appLogger.Info("server listening", "addr", addr, "component", "server")
		if err := app.Listen(addr); err != nil {
			serverErr <- err
		}
	}()

	// Esperar a que llegue una señal de apagado o un error fatal del servidor.
	select {
	case sig := <-quit:
		appLogger.Info("received signal, initiating graceful shutdown", "signal", sig.String(), "component", "server")
	case err := <-serverErr:
		appLogger.Error("server error", "error", err, "component", "server")
	}

	// Crear un contexto con timeout para el apagado ordenado.
	// Si el servidor no termina en GracefulShutdownTimeout, se fuerza el cierre.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer cancel()

	// Iniciar el apagado de Fiber en una goroutine para poder aplicar el timeout.
	done := make(chan struct{})
	go func() {
		if err := app.Shutdown(); err != nil {
			appLogger.Error("shutdown error", "error", err, "component", "server")
		}
		close(done)
	}()

	// Esperar a que el servidor termine o a que expire el timeout de apagado.
	select {
	case <-shutdownCtx.Done():
		appLogger.Warn("graceful shutdown timed out", "timeout", cfg.Server.GracefulShutdownTimeout.String(), "component", "server")
	case <-done:
		appLogger.Info("server shut down gracefully", "component", "server")
	}
}

// healthHandler retorna un handler Fiber que verifica el estado de las dependencias críticas
// del servicio: PostgreSQL y Redis.
//
// Se usa en el endpoint GET /health, el cual no requiere autenticación y puede ser
// consultado por balanceadores de carga o sistemas de monitoreo para determinar
// si la instancia está en condiciones de recibir tráfico.
//
// El handler realiza un ping a cada dependencia con un timeout de 3 segundos.
// Si alguna falla, el estado del servicio es "unhealthy" y retorna HTTP 503.
// Si todas responden correctamente, retorna HTTP 200 con estado "healthy".
//
// Parámetros:
//   - pool: pool de conexiones a PostgreSQL para el ping de verificación.
//   - rdb: cliente Redis para el ping de verificación.
//
// Retorna un fiber.Handler que responde con JSON:
//
//	{"status": "healthy"|"unhealthy", "version": "1.0.0", "checks": {"postgresql": "ok", "redis": "ok"}}
//
// @Summary     Estado del servicio
// @Description Verifica el estado de salud del servicio y sus dependencias (PostgreSQL y Redis).
// @Tags        Sistema
// @Produce     json
// @Success     200 {object} handler.SwaggerHealthResponse "Servicio operativo"
// @Failure     503 {object} handler.SwaggerHealthResponse "Servicio degradado"
// @Router      /health [get]
func healthHandler(pool *pgxpool.Pool, rdb *goredis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Timeout de 3 segundos para los pings; si una dependencia tarda más, se considera caída.
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		healthy := true
		checks := fiber.Map{}

		// Verificar PostgreSQL.
		if err := pool.Ping(ctx); err != nil {
			healthy = false
			checks["postgresql"] = fmt.Sprintf("error: %v", err)
		} else {
			checks["postgresql"] = "ok"
		}

		// Verificar Redis.
		if err := rdb.Ping(ctx).Err(); err != nil {
			healthy = false
			checks["redis"] = fmt.Sprintf("error: %v", err)
		} else {
			checks["redis"] = "ok"
		}

		status := "healthy"
		httpStatus := fiber.StatusOK
		if !healthy {
			status = "unhealthy"
			httpStatus = fiber.StatusServiceUnavailable
		}

		return c.Status(httpStatus).JSON(fiber.Map{
			"status":  status,
			"version": "1.0.0",
			"checks":  checks,
		})
	}
}

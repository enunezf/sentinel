package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/enunezf/sentinel/internal/bootstrap"
	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/handler"
	"github.com/enunezf/sentinel/internal/middleware"
	pgrepository "github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepository "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/service"
	"github.com/enunezf/sentinel/internal/token"
)

func main() {
	// Load configuration.
	configPath := "config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}

	// -----------------------------------------------------------------------
	// PostgreSQL connection pool
	// -----------------------------------------------------------------------
	pgCfg, err := pgxpool.ParseConfig(cfg.Database.DSN())
	if err != nil {
		log.Fatalf("FATAL: cannot parse database DSN: %v", err)
	}
	pgCfg.MaxConns = int32(cfg.Database.MaxOpenConns)
	pgCfg.MinConns = int32(cfg.Database.MaxIdleConns)
	pgCfg.MaxConnLifetime = cfg.Database.ConnMaxLifetime

	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		log.Fatalf("FATAL: cannot create PostgreSQL pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("FATAL: cannot connect to PostgreSQL: %v", err)
	}
	log.Println("INFO: PostgreSQL connection pool established")

	// -----------------------------------------------------------------------
	// Redis connection
	// -----------------------------------------------------------------------
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Printf("WARN: error closing Redis client: %v", err)
		}
	}()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("FATAL: cannot connect to Redis: %v", err)
	}
	log.Println("INFO: Redis connection established")

	// -----------------------------------------------------------------------
	// Token manager (RSA keys)
	// -----------------------------------------------------------------------
	tokenMgr, err := token.NewManager(cfg.JWT.PrivateKeyPath, cfg.JWT.PublicKeyPath)
	if err != nil {
		log.Fatalf("FATAL: cannot load RSA keys: %v", err)
	}
	log.Println("INFO: RSA key pair loaded")

	// -----------------------------------------------------------------------
	// Repositories
	// -----------------------------------------------------------------------
	userRepo := pgrepository.NewUserRepository(pool)
	appRepo := pgrepository.NewApplicationRepository(pool)
	refreshPGRepo := pgrepository.NewRefreshTokenRepository(pool)
	auditRepo := pgrepository.NewAuditRepository(pool)
	roleRepo := pgrepository.NewRoleRepository(pool)
	permRepo := pgrepository.NewPermissionRepository(pool)
	ccRepo := pgrepository.NewCostCenterRepository(pool)
	userRoleRepo := pgrepository.NewUserRoleRepository(pool)
	userPermRepo := pgrepository.NewUserPermissionRepository(pool)
	userCCRepo := pgrepository.NewUserCostCenterRepository(pool)
	pwdHistRepo := pgrepository.NewPasswordHistoryRepository(pool)

	refreshRedisRepo := redisrepository.NewRefreshTokenRepository(rdb)
	authzCache := redisrepository.NewAuthzCache(rdb)

	// -----------------------------------------------------------------------
	// Services
	// -----------------------------------------------------------------------
	auditSvc := service.NewAuditService(auditRepo)
	defer auditSvc.Close()

	authSvc := service.NewAuthService(
		userRepo, appRepo, refreshPGRepo, refreshRedisRepo,
		pwdHistRepo, userRoleRepo, tokenMgr, auditSvc, cfg,
	)

	authzSvc := service.NewAuthzService(
		appRepo, userRoleRepo, userPermRepo, userCCRepo,
		permRepo, roleRepo, ccRepo, authzCache, tokenMgr, auditSvc,
	)

	userSvc := service.NewUserService(
		userRepo, userRoleRepo, userPermRepo, userCCRepo,
		refreshPGRepo, pwdHistRepo, appRepo, auditSvc, cfg,
	)

	roleSvc := service.NewRoleService(roleRepo, permRepo, appRepo, authzCache, auditSvc)
	permSvc := service.NewPermissionService(permRepo, appRepo, authzCache, auditSvc)
	ccSvc := service.NewCostCenterService(ccRepo, appRepo, authzCache, auditSvc)

	// -----------------------------------------------------------------------
	// Bootstrap
	// -----------------------------------------------------------------------
	initializer := bootstrap.NewInitializer(appRepo, userRepo, roleRepo, permRepo, userRoleRepo, auditRepo, cfg)
	if err := initializer.Initialize(ctx); err != nil {
		log.Fatalf("FATAL: bootstrap failed: %v", err)
	}

	// -----------------------------------------------------------------------
	// Handlers
	// -----------------------------------------------------------------------
	authHandler := handler.NewAuthHandler(authSvc, tokenMgr)
	authzHandler := handler.NewAuthzHandler(authzSvc)
	adminHandler := handler.NewAdminHandler(userSvc, roleSvc, permSvc, ccSvc, auditRepo, appRepo)

	// -----------------------------------------------------------------------
	// Fiber application
	// -----------------------------------------------------------------------
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "internal server error"
			var fiberErr *fiber.Error
			if e, ok := err.(*fiber.Error); ok {
				fiberErr = e
				code = fiberErr.Code
				msg = fiberErr.Message
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

	// Global middleware.
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: `{"time":"${time}","status":${status},"latency":"${latency}","method":"${method}","path":"${path}"}` + "\n",
	}))
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.AuditContext())

	// Middleware shortcuts.
	appKeyMW := middleware.AppKey(appRepo)
	jwtMW := middleware.JWTAuth(tokenMgr)
	requirePerm := func(code string) fiber.Handler {
		return middleware.RequirePermission(authzSvc, code)
	}

	// -----------------------------------------------------------------------
	// Routes
	// -----------------------------------------------------------------------

	// Public endpoints (no auth).
	app.Get("/health", healthHandler(pool, rdb))
	app.Get("/.well-known/jwks.json", authHandler.JWKS)

	// Auth endpoints (app_key required).
	auth := app.Group("/auth", appKeyMW)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	auth.Post("/logout", jwtMW, authHandler.Logout)
	auth.Post("/change-password", jwtMW, authHandler.ChangePassword)

	// Authz endpoints.
	authz := app.Group("/authz")
	authz.Post("/verify", appKeyMW, jwtMW, authzHandler.Verify)
	authz.Get("/me/permissions", jwtMW, authzHandler.MePermissions)
	authz.Get("/permissions-map", appKeyMW, authzHandler.PermissionsMap)
	authz.Get("/permissions-map/version", appKeyMW, authzHandler.PermissionsMapVersion)

	// Admin endpoints (app_key + jwt + permission).
	admin := app.Group("/admin", appKeyMW, jwtMW)

	// Users.
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

	// Roles.
	admin.Get("/roles", requirePerm("admin.roles.read"), adminHandler.ListRoles)
	admin.Post("/roles", requirePerm("admin.roles.write"), adminHandler.CreateRole)
	admin.Get("/roles/:id", requirePerm("admin.roles.read"), adminHandler.GetRole)
	admin.Put("/roles/:id", requirePerm("admin.roles.write"), adminHandler.UpdateRole)
	admin.Delete("/roles/:id", requirePerm("admin.roles.write"), adminHandler.DeleteRole)
	admin.Post("/roles/:id/permissions", requirePerm("admin.permissions.write"), adminHandler.AddRolePermission)
	admin.Delete("/roles/:id/permissions/:pid", requirePerm("admin.permissions.write"), adminHandler.RemoveRolePermission)

	// Permissions.
	admin.Get("/permissions", requirePerm("admin.permissions.read"), adminHandler.ListPermissions)
	admin.Post("/permissions", requirePerm("admin.permissions.write"), adminHandler.CreatePermission)
	admin.Delete("/permissions/:id", requirePerm("admin.permissions.write"), adminHandler.DeletePermission)

	// Cost centers.
	admin.Get("/cost-centers", requirePerm("admin.cost_centers.read"), adminHandler.ListCostCenters)
	admin.Post("/cost-centers", requirePerm("admin.cost_centers.write"), adminHandler.CreateCostCenter)
	admin.Put("/cost-centers/:id", requirePerm("admin.cost_centers.write"), adminHandler.UpdateCostCenter)

	// Applications.
	admin.Get("/applications", requirePerm("admin.system.manage"), adminHandler.ListApplications)
	admin.Post("/applications", requirePerm("admin.system.manage"), adminHandler.CreateApplication)
	admin.Get("/applications/:id", requirePerm("admin.system.manage"), adminHandler.GetApplication)
	admin.Put("/applications/:id", requirePerm("admin.system.manage"), adminHandler.UpdateApplication)
	admin.Post("/applications/:id/rotate-key", requirePerm("admin.system.manage"), adminHandler.RotateApplicationKey)

	// Audit logs.
	admin.Get("/audit-logs", requirePerm("admin.audit.read"), adminHandler.ListAuditLogs)

	// -----------------------------------------------------------------------
	// Graceful shutdown
	// -----------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Printf("INFO: server listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			serverErr <- err
		}
	}()

	select {
	case sig := <-quit:
		log.Printf("INFO: received signal %s, initiating graceful shutdown", sig)
	case err := <-serverErr:
		log.Printf("ERROR: server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		if err := app.Shutdown(); err != nil {
			log.Printf("ERROR: shutdown error: %v", err)
		}
		close(done)
	}()

	select {
	case <-shutdownCtx.Done():
		log.Printf("WARN: graceful shutdown timed out after %s", cfg.Server.GracefulShutdownTimeout)
	case <-done:
		log.Println("INFO: server shut down gracefully")
	}
}

// healthHandler returns a Fiber handler that checks PostgreSQL and Redis liveness.
func healthHandler(pool *pgxpool.Pool, rdb *goredis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		healthy := true
		checks := fiber.Map{}

		if err := pool.Ping(ctx); err != nil {
			healthy = false
			checks["postgresql"] = fmt.Sprintf("error: %v", err)
		} else {
			checks["postgresql"] = "ok"
		}

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

# Plan de Trabajo — Proyecto Sentinel
## Auth Service: Servicio de Autenticación y Autorización

**Versión:** 1.0.0
**Fecha:** Febrero 2026
**Equipo:** Transformación Digital — Sodexo Chile

---

## Resumen del Proyecto

**Sentinel** es un microservicio independiente escrito en Go que actúa como fuente de verdad central de identidad y autorización para múltiples aplicaciones de Sodexo Chile. Centraliza la autenticación de usuarios, la emisión de tokens JWT RS256 y la verificación de permisos mediante un modelo híbrido RBAC + permisos individuales + centros de costo (CeCos).

### Características Clave

| Aspecto | Detalle |
|---|---|
| Lenguaje | Go 1.22+ |
| Base de datos | PostgreSQL 15+ |
| Caché | Redis 7+ |
| Tokens | JWT RS256 (60 min) + Refresh Token (7/30 días) |
| Autorización | RBAC + permisos individuales + CeCos |
| Multi-aplicación | Sí (web, móvil, escritorio) |
| Backends consumidores | .NET Core, Python, Go |
| Usuarios soportados | Hasta 2.000 usuarios activos |
| Despliegue destino | Azure (Container Apps / AKS) |

---

## Arquitectura General

El servicio expone tres grupos de endpoints:
- **`/auth/*`** — Autenticación: login, refresh, logout, cambio de contraseña.
- **`/authz/*`** — Autorización: verificación de permisos, mapa global, contexto de usuario.
- **`/admin/*`** — Administración: gestión de usuarios, roles, permisos y CeCos.

Los backends consumidores verifican tokens **localmente** usando la clave pública RSA expuesta en `/.well-known/jwks.json`, sin llamadas adicionales al Auth Service por request. El mapa global de permisos firmado se cachea en memoria con TTL de 5 minutos y polling de versión cada 2 minutos.

### Estructura del Proyecto

```
sentinel/
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── domain/          # user, role, permission, cost_center, audit
│   ├── repository/      # postgres/, redis/
│   ├── service/         # auth, authz, user, role, audit
│   ├── handler/         # auth, authz, admin
│   ├── middleware/       # jwt, app_key, permission, audit
│   ├── token/           # jwt.go
│   └── bootstrap/       # initializer.go
├── migrations/
├── docker-compose.yml
├── Dockerfile
└── config.yaml
```

---

## Modelo de Datos (10 tablas)

| Tabla | Propósito |
|---|---|
| `applications` | Registro de aplicaciones cliente (multi-tenancy) |
| `users` | Credenciales, estado de cuenta, bloqueo por intentos |
| `permissions` | Catálogo de permisos por app (`módulo.recurso.acción`) |
| `cost_centers` | Centros de costo para control de acceso granular |
| `roles` | Agrupadores dinámicos de permisos |
| `role_permissions` | Relación N:M entre roles y permisos |
| `user_roles` | Asignación de roles a usuarios con vigencia temporal |
| `user_permissions` | Permisos individuales con vigencia temporal |
| `user_cost_centers` | Acceso de usuarios a CeCos específicos |
| `refresh_tokens` | Tokens de renovación (hash bcrypt en Redis + PG) |
| `audit_logs` | Registro inmutable de eventos de seguridad |

---

## Fases del Proyecto

---

## Fase 1 — Core (Semanas 1–2)

**Objetivo:** Infraestructura base, autenticación funcional y emisión de JWT RS256.

### 1.1 Configuración del Proyecto Go

- [ ] Inicializar módulo Go (`go mod init`)
- [ ] Definir estructura de directorios del proyecto (`cmd/`, `internal/`, `migrations/`)
- [ ] Elegir framework HTTP: **Fiber v2** o **Chi** (decisión de equipo)
- [ ] Configurar linter (`golangci-lint`) y formateador (`gofmt`)
- [ ] Configurar `Makefile` con targets: `build`, `run`, `test`, `lint`, `migrate`

### 1.2 Infraestructura Local (Docker Compose)

- [ ] Crear `docker-compose.yml` con servicios: `auth-service`, `postgres`, `redis`
- [ ] Configurar volúmenes persistentes para PostgreSQL y Redis
- [ ] Crear `Dockerfile` multi-stage para el binario Go
- [ ] Crear archivo `config.yaml` con estructura completa (server, database, redis, jwt, security, bootstrap, logging)
- [ ] Implementar carga de configuración desde variables de entorno + YAML (`internal/config/`)

### 1.3 Migraciones de Base de Datos

- [ ] Elegir herramienta de migraciones (`golang-migrate` o `goose`)
- [ ] Crear migración `001_initial_schema.sql`:
  - [ ] Tabla `applications`
  - [ ] Tabla `users`
  - [ ] Tabla `permissions`
  - [ ] Tabla `cost_centers`
  - [ ] Tabla `roles`
  - [ ] Tabla `role_permissions`
  - [ ] Tabla `user_roles`
  - [ ] Tabla `user_permissions`
  - [ ] Tabla `user_cost_centers`
  - [ ] Tabla `refresh_tokens`
  - [ ] Tabla `audit_logs`
  - [ ] Índices de `audit_logs` (`user_id`, `actor_id`, `event_type`, `application_id`)
- [ ] Crear migración `002_seed_permissions.sql` con permisos base de administración
- [ ] Integrar ejecución automática de migraciones al arranque del servicio

### 1.4 Modelos de Dominio

- [ ] `internal/domain/user.go` — struct `User`, estados de cuenta, política de contraseñas
- [ ] `internal/domain/role.go` — struct `Role`, `UserRole` (con `valid_from`, `valid_until`)
- [ ] `internal/domain/permission.go` — struct `Permission`, `UserPermission` (con vigencia)
- [ ] `internal/domain/cost_center.go` — struct `CostCenter`, `UserCostCenter`
- [ ] `internal/domain/audit.go` — struct `AuditLog`, constantes de tipos de evento

### 1.5 Repositorios (PostgreSQL)

- [ ] `internal/repository/postgres/user_repository.go` — CRUD + búsqueda por username/email
- [ ] `internal/repository/postgres/application_repository.go` — CRUD + búsqueda por slug/secret_key

### 1.6 Repositorios (Redis)

- [ ] `internal/repository/redis/refresh_token_repository.go` — Set/Get/Delete con TTL
- [ ] Configurar pool de conexiones Redis

### 1.7 Generación y Validación de JWT RS256

- [ ] `internal/token/jwt.go`:
  - [ ] Generar par de claves RSA (o leer desde archivo / Azure Key Vault)
  - [ ] Función `GenerateAccessToken(user, app, roles) (string, error)` — RS256, 60 min, incluye `jti`
  - [ ] Función `ValidateAccessToken(tokenString) (JWTClaims, error)`
  - [ ] Función `BuildJWKS() (JWKS, error)` — expone clave pública con `kid`

### 1.8 Servicio de Autenticación

- [ ] `internal/service/auth_service.go`:
  - [ ] `Login(username, password, appSlug, deviceInfo) (TokenPair, error)`
    - Verificar aplicación por `X-App-Key`
    - Validar credenciales (`bcrypt.CompareHashAndPassword`)
    - Verificar estado de cuenta (`is_active`, `locked_until`)
    - Registrar intento fallido / exitoso en auditoría
    - Generar `access_token` (JWT RS256) y `refresh_token` (UUID v4, hash bcrypt)
    - Persistir refresh token en Redis y PostgreSQL
  - [ ] `Refresh(refreshToken, appSlug) (TokenPair, error)`
    - Buscar hash en Redis, verificar no revocado ni expirado
    - Rotación: invalidar token anterior, emitir nuevo par
  - [ ] `Logout(userID, appID) error`
    - Revocar refresh token activo del usuario en la aplicación
  - [ ] `ChangePassword(userID, currentPassword, newPassword) error`
    - Validar contraseña actual
    - Validar política (10 chars, mayúscula, número, símbolo)
    - Verificar historial de últimas 5 contraseñas
    - Hashear con bcrypt costo 12

### 1.9 Handlers de Autenticación

- [ ] `internal/handler/auth_handler.go`:
  - [ ] `POST /auth/login`
  - [ ] `POST /auth/refresh`
  - [ ] `POST /auth/logout` (requiere JWT válido)
  - [ ] `POST /auth/change-password` (requiere JWT válido)

### 1.10 Middleware Base

- [ ] `internal/middleware/app_key_middleware.go` — Valida `X-App-Key` en cada request
- [ ] `internal/middleware/jwt_middleware.go` — Valida Bearer token y extrae claims

### 1.11 Endpoints de Utilidad

- [ ] `GET /health` — Health check (sin autenticación)
- [ ] `GET /.well-known/jwks.json` — Expone clave pública RSA para consumidores

### 1.12 Bootstrap del Sistema

- [ ] `internal/bootstrap/initializer.go`:
  - [ ] Detectar BD vacía (sin aplicaciones ni usuarios)
  - [ ] Crear aplicación `system` con slug `system`
  - [ ] Crear rol `admin` (`is_system = true`) con todos los permisos de gestión
  - [ ] Crear usuario administrador inicial desde `BOOTSTRAP_ADMIN_USER` / `BOOTSTRAP_ADMIN_PASSWORD`
  - [ ] Forzar `must_change_pwd = true` en primer login
  - [ ] Registrar evento `SYSTEM_BOOTSTRAP` en auditoría
  - [ ] Guardar flag de bootstrap completado (evitar re-ejecución)

### 1.13 Servidor HTTP

- [ ] `cmd/server/main.go` — Punto de entrada: carga config, ejecuta bootstrap, inicia servidor
- [ ] Configurar graceful shutdown (15 s)
- [ ] Agregar headers de seguridad a todas las respuestas: `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`

### Criterios de Aceptación — Fase 1

- [ ] `docker-compose up` levanta los tres servicios sin errores
- [ ] Las migraciones se ejecutan automáticamente al arrancar
- [ ] El bootstrap crea el admin inicial en una BD vacía y no se re-ejecuta
- [ ] `POST /auth/login` con credenciales válidas retorna `access_token` (JWT RS256) y `refresh_token`
- [ ] `POST /auth/refresh` rota correctamente el refresh token
- [ ] `GET /.well-known/jwks.json` retorna la clave pública RSA
- [ ] Un JWT generado puede verificarse manualmente con la clave pública del endpoint JWKS

---

## Fase 2 — Autorización (Semanas 3–4)

**Objetivo:** Motor RBAC completo, soporte de vigencia temporal, control por CeCo y endpoints de autorización.

### 2.1 Repositorios Adicionales (PostgreSQL)

- [ ] `postgres/permission_repository.go` — CRUD, listar por aplicación
- [ ] `postgres/role_repository.go` — CRUD, listar con sus permisos
- [ ] `postgres/user_role_repository.go` — Asignación/revocación con `valid_from`/`valid_until`, consulta de roles vigentes
- [ ] `postgres/user_permission_repository.go` — Asignación/revocación con vigencia temporal
- [ ] `postgres/cost_center_repository.go` — CRUD
- [ ] `postgres/user_cost_center_repository.go` — Asignación de CeCos a usuarios

### 2.2 Servicio de Autorización

- [ ] `internal/service/authz_service.go`:
  - [ ] `GetUserContext(userID, appID) (UserContext, error)` — Calcula permisos efectivos (roles vigentes + `extra_permissions` + CeCos)
  - [ ] `VerifyPermission(userID, appID, permission, costCenterID?) (bool, error)`
    - Recuperar contexto del usuario
    - Implementar algoritmo `HasPermission` (ver spec §5.3)
  - [ ] `GetPermissionsMap(appID) (PermissionsMap, error)` — Construye y firma el mapa global
  - [ ] `GetPermissionsMapVersion(appID) (string, error)` — Hash del estado actual del mapa

### 2.3 Firma RSA del Mapa de Permisos

- [ ] Implementar `canonicalJSON(payload)` — serialización con claves ordenadas lexicográficamente, sin espacios
- [ ] `SignPermissionsMap(payload, privateKey) (string, error)` — RSA-SHA256, retorna base64url
- [ ] `VerifyPermissionsMap(raw []byte, pubKey) error` — Verificación de referencia (para tests y consumidores Go)

### 2.4 Caché de Autorización (Redis)

- [ ] `redis/authz_cache.go`:
  - [ ] Cachear `UserContext` por `jti` con TTL de 60 min
  - [ ] Invalidar caché de usuario cuando se modifiquen sus roles, permisos o CeCos

### 2.5 Handlers de Autorización

- [ ] `internal/handler/authz_handler.go`:
  - [ ] `POST /authz/verify` — Verificación delegada (para scripts/automatización)
  - [ ] `GET /authz/me/permissions` — Contexto completo del usuario autenticado
  - [ ] `GET /authz/permissions-map` — Mapa global firmado para backends consumidores
  - [ ] `GET /authz/permissions-map/version` — Endpoint liviano de polling de versión

### 2.6 Middleware de Permisos

- [ ] `internal/middleware/permission_middleware.go` — Middleware declarativo para proteger rutas del `/admin/*` con permisos requeridos

### 2.7 Soporte de Vigencia Temporal

- [ ] Lógica para evaluar `valid_from` y `valid_until` en `user_roles` y `user_permissions`
- [ ] Job o trigger para invalidar caché de usuarios cuando expiran asignaciones temporales (alternativa: evaluar vigencia en runtime)

### Criterios de Aceptación — Fase 2

- [ ] `GET /authz/me/permissions` retorna roles vigentes, extra_permissions y CeCos del usuario
- [ ] `POST /authz/verify` responde `allowed: true/false` correctamente para distintos escenarios
- [ ] `GET /authz/permissions-map` retorna el mapa firmado; la firma verifica con la clave pública del JWKS
- [ ] Un rol temporal expirado **no** otorga permisos
- [ ] Un permiso individual con `valid_until` pasado **no** es efectivo
- [ ] El polling de versión detecta cambios en el mapa al modificar permisos/roles

---

## Fase 3 — Administración y Auditoría (Semanas 5–6)

**Objetivo:** API de administración completa, sistema de auditoría inmutable y protección contra fuerza bruta.

### 3.1 Servicio de Administración de Usuarios

- [ ] `internal/service/user_service.go`:
  - [ ] `CreateUser(data) (User, error)` — Hashear contraseña, generar UUID, forzar `must_change_pwd`
  - [ ] `UpdateUser(id, data) (User, error)`
  - [ ] `DeactivateUser(id) error`
  - [ ] `UnlockUser(id) error` — Resetear `failed_attempts` y `locked_until`
  - [ ] `ResetPassword(id) error` — Generar contraseña temporal, forzar cambio en próximo login
  - [ ] `AssignRole(userID, roleID, appID, validUntil?, grantedBy) error`
  - [ ] `RevokeRole(userID, roleID) error`
  - [ ] `AssignPermission(userID, permissionID, appID, validUntil?, grantedBy) error`
  - [ ] `RevokePermission(userID, permissionID) error`
  - [ ] `AssignCostCenters(userID, costCenterIDs[], appID, grantedBy) error`

### 3.2 Servicio de Administración de Roles

- [ ] `internal/service/role_service.go`:
  - [ ] `CreateRole(name, description, appID) (Role, error)`
  - [ ] `UpdateRole(id, data) (Role, error)`
  - [ ] `DeactivateRole(id) error`
  - [ ] `AssignPermissionToRole(roleID, permissionID) error`
  - [ ] `RemovePermissionFromRole(roleID, permissionID) error`

### 3.3 Servicio de Administración de Permisos y CeCos

- [ ] `internal/service/permission_service.go`:
  - [ ] `CreatePermission(code, description, scopeType, appID) (Permission, error)` — Validar convención `módulo.recurso.acción`
  - [ ] `DeletePermission(id) error`
- [ ] `internal/service/cost_center_service.go`:
  - [ ] `CreateCostCenter(code, name, appID) (CostCenter, error)`
  - [ ] `UpdateCostCenter(id, data) (CostCenter, error)`

### 3.4 Handler de Administración

- [ ] `internal/handler/admin_handler.go` — Todos los endpoints requieren permiso `admin.system.manage`:

  **Gestión de roles:**
  - [ ] `GET /admin/roles`
  - [ ] `POST /admin/roles`
  - [ ] `GET /admin/roles/:id`
  - [ ] `PUT /admin/roles/:id`
  - [ ] `DELETE /admin/roles/:id`
  - [ ] `POST /admin/roles/:id/permissions`
  - [ ] `DELETE /admin/roles/:id/permissions/:pid`

  **Gestión de usuarios:**
  - [ ] `GET /admin/users`
  - [ ] `POST /admin/users`
  - [ ] `GET /admin/users/:id`
  - [ ] `PUT /admin/users/:id`
  - [ ] `POST /admin/users/:id/roles`
  - [ ] `DELETE /admin/users/:id/roles/:rid`
  - [ ] `POST /admin/users/:id/permissions`
  - [ ] `DELETE /admin/users/:id/permissions/:pid`
  - [ ] `POST /admin/users/:id/cost-centers`
  - [ ] `POST /admin/users/:id/unlock`
  - [ ] `POST /admin/users/:id/reset-password`

  **Gestión de permisos:**
  - [ ] `GET /admin/permissions`
  - [ ] `POST /admin/permissions`
  - [ ] `DELETE /admin/permissions/:id`

  **Gestión de CeCos:**
  - [ ] `GET /admin/cost-centers`
  - [ ] `POST /admin/cost-centers`
  - [ ] `PUT /admin/cost-centers/:id`

  **Auditoría:**
  - [ ] `GET /admin/audit-logs` (filtros: `user_id`, `actor_id`, `event_type`, `from_date`, `to_date`, `application_id`, `success`)

### 3.5 Sistema de Auditoría

- [ ] `internal/service/audit_service.go`:
  - [ ] `LogEvent(eventType, appID?, userID?, actorID?, resourceType?, resourceID?, oldValue?, newValue?, ip, userAgent, success, errorMsg?) error`
  - [ ] Garantizar inmutabilidad: sin UPDATE ni DELETE sobre `audit_logs`
- [ ] `internal/middleware/audit_middleware.go` — Registro automático de eventos en endpoints críticos

- [ ] Implementar todos los tipos de evento definidos en la spec:
  - [ ] Autenticación: `AUTH_LOGIN_SUCCESS`, `AUTH_LOGIN_FAILED`, `AUTH_LOGOUT`, `AUTH_TOKEN_REFRESHED`, `AUTH_PASSWORD_CHANGED`, `AUTH_PASSWORD_RESET`, `AUTH_ACCOUNT_LOCKED`
  - [ ] Autorización: `AUTHZ_PERMISSION_GRANTED`, `AUTHZ_PERMISSION_DENIED`
  - [ ] Gestión de usuarios: `USER_CREATED`, `USER_UPDATED`, `USER_DEACTIVATED`, `USER_UNLOCKED`
  - [ ] Gestión de roles: `ROLE_CREATED`, `ROLE_UPDATED`, `ROLE_DELETED`, `ROLE_PERMISSION_ASSIGNED`, `ROLE_PERMISSION_REVOKED`
  - [ ] Asignaciones: `USER_ROLE_ASSIGNED`, `USER_ROLE_REVOKED`, `USER_PERMISSION_ASSIGNED`, `USER_PERMISSION_REVOKED`, `USER_COST_CENTER_ASSIGNED`

### 3.6 Protección contra Fuerza Bruta

- [ ] Implementar contador de intentos fallidos en `users.failed_attempts`
- [ ] Bloqueo automático a los 5 intentos fallidos consecutivos por 15 minutos (`locked_until`)
- [ ] Bloqueo manual tras 3 bloqueos en el mismo día (requiere desbloqueo por admin)
- [ ] Registrar cada intento fallido como `AUTH_LOGIN_FAILED` con IP y user-agent
- [ ] Registrar `AUTH_ACCOUNT_LOCKED` al activar el bloqueo

### 3.7 Política de Contraseñas

- [ ] Validar longitud mínima de 10 caracteres
- [ ] Requerir al menos: 1 mayúscula, 1 número, 1 símbolo
- [ ] Historial de últimas 5 contraseñas: almacenar hashes anteriores y rechazar reutilización

### Criterios de Aceptación — Fase 3

- [ ] El admin puede crear usuarios, roles y asignar permisos desde la API
- [ ] Después de 5 intentos fallidos, la cuenta se bloquea automáticamente 15 minutos
- [ ] Los logs de auditoría son inmutables (no existe endpoint de DELETE/UPDATE sobre `audit_logs`)
- [ ] `GET /admin/audit-logs` permite filtrar por usuario, actor, tipo de evento y rango de fechas
- [ ] La política de contraseñas se aplica en creación y en cambio de contraseña

---

## Fase 4 — Calidad y Hardening (Semanas 7–8)

**Objetivo:** Cobertura de tests ≥ 80 %, documentación, load testing y despliegue en Azure staging.

### 4.1 Tests de Integración

- [ ] Configurar entorno de test con PostgreSQL y Redis en Docker (Testcontainers o `docker-compose.test.yml`)
- [ ] Tests de integración para autenticación:
  - [ ] Login exitoso
  - [ ] Login con credenciales incorrectas
  - [ ] Login con cuenta bloqueada / inactiva
  - [ ] Refresh token válido
  - [ ] Refresh token expirado / revocado
  - [ ] Logout invalida el refresh token
  - [ ] Cambio de contraseña: exitoso, contraseña incorrecta, política no cumplida, reutilización
- [ ] Tests de integración para autorización:
  - [ ] `HasPermission` — permiso por rol, por `extra_permission`, con CeCo válido/inválido
  - [ ] Rol temporal vigente vs. expirado
  - [ ] Permiso individual vigente vs. expirado
  - [ ] Firma del mapa de permisos: verificación exitosa, fallo ante modificación del payload
- [ ] Tests de integración para administración:
  - [ ] Crear/actualizar/desactivar usuarios
  - [ ] Asignar/revocar roles y permisos
  - [ ] Consultar audit logs con filtros
- [ ] Tests para bootstrap: BD vacía crea admin; segunda ejecución no duplica datos
- [ ] Alcanzar cobertura ≥ 80 %

### 4.2 Tests Unitarios

- [ ] `token/jwt.go` — Generación y validación de JWT RS256
- [ ] `service/authz_service.go` — Algoritmo `HasPermission` con todos los casos borde
- [ ] `bootstrap/initializer.go` — Idempotencia del bootstrap
- [ ] Función `canonicalJSON` — Serialización determinista
- [ ] `VerifyPermissionsMap` — Verificación de firma

### 4.3 Documentación OpenAPI / Swagger

- [ ] Generar especificación OpenAPI 3.0 (`swagger.yaml` o anotaciones en código)
- [ ] Documentar todos los endpoints con request, response y errores posibles
- [ ] Incluir ejemplos de payload (login, refresh, verify, permissions-map)
- [ ] Publicar Swagger UI en `/docs` (solo en entornos non-production)

### 4.4 Load Testing

- [ ] Crear escenarios con **Grafana k6** o **hey**:
  - [ ] `POST /auth/login` — objetivo: < 200 ms p95 con carga sostenida
  - [ ] `POST /authz/verify` — objetivo: < 50 ms p95
  - [ ] `GET /authz/permissions-map` — objetivo: estabilidad bajo múltiples backends en polling
- [ ] Ajustar pool de conexiones PostgreSQL y Redis según resultados
- [ ] Documentar resultados y cuellos de botella encontrados

### 4.5 Revisión de Seguridad

- [ ] Auditar manejo de secretos: ningún secret en código ni logs
- [ ] Verificar headers de seguridad en todas las respuestas (`HSTS`, `X-Content-Type-Options`, `X-Frame-Options`)
- [ ] Revisar que `bcrypt` costo ≥ 12 en todos los contextos de hashing
- [ ] Verificar que los refresh tokens se almacenan únicamente como hash (nunca en plaintext)
- [ ] Confirmar que `audit_logs` no tiene endpoints de modificación
- [ ] Revisar inputs de usuario: prevenir SQL injection (uso de queries parametrizadas), XSS en campos de texto
- [ ] Confirmar TLS 1.2+ en despliegue

### 4.6 Rotación de Claves RSA (Operativo)

- [ ] Integrar lectura de clave privada RSA desde **Azure Key Vault**
- [ ] Implementar soporte de múltiples `kid` en `/.well-known/jwks.json` para rotación sin downtime
- [ ] Documentar procedimiento operativo de rotación

### 4.7 Despliegue en Azure (Staging)

- [ ] Crear `Dockerfile` de producción (multi-stage, imagen mínima)
- [ ] Configurar variables de entorno y secretos en Azure (Key Vault references)
- [ ] Definir manifiestos de despliegue para Azure Container Apps o AKS
- [ ] Configurar health check en el orquestador apuntando a `GET /health`
- [ ] Validar despliegue en entorno de staging con smoke tests

### 4.8 Observabilidad

- [ ] Configurar logging estructurado en JSON (`level: info`)
- [ ] Agregar métricas básicas de latencia por endpoint (Prometheus o Azure Monitor)
- [ ] Definir alertas para: tasa de errores > 1 %, latencia p95 > SLA, intentos de fuerza bruta masivos

### Criterios de Aceptación — Fase 4

- [ ] Cobertura de tests ≥ 80 % (`go test ./... -cover`)
- [ ] Documentación OpenAPI accesible y sin endpoints sin documentar
- [ ] Load test: `POST /auth/login` < 200 ms p95, `POST /authz/verify` < 50 ms p95
- [ ] Despliegue en staging exitoso con smoke tests pasando
- [ ] Sin secretos expuestos en logs ni en el código fuente

---

## SLAs del Servicio

| Métrica | Objetivo |
|---|---|
| Latencia `POST /auth/login` | < 200 ms (p95) |
| Latencia `POST /authz/verify` | < 50 ms (p95) |
| Latencia verificación JWT local (backend) | < 5 ms (en memoria) |
| Usuarios concurrentes soportados | 500+ |
| Disponibilidad mensual | 99.5 % |
| RTO (tiempo de recuperación) | < 5 minutos |

---

## Alcance Excluido en v1.0

Los siguientes elementos **no forman parte del alcance** de este plan y se abordarán en versiones futuras:

- SSO / OAuth2 externo
- MFA / 2FA
- Federación de identidad
- API Keys para integraciones B2B
- Revocación inmediata de access tokens (antes de expiración)

---

*Fin del Plan de Trabajo v1.0.0 — Proyecto Sentinel*

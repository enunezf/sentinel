# Sentinel Test Suite

## Tests Unitarios

Ejecutar todos los tests unitarios con cobertura:

```bash
cd /home/enunez/proyectos/github.com/enunezf/sentinel
go test ./internal/... -v -cover
```

Ejecutar por paquete:

```bash
# Token / JWT
go test ./internal/token/... -v -cover

# Auth service (login, refresh, logout, change-password)
go test ./internal/service/... -v -cover -run TestLogin
go test ./internal/service/... -v -cover -run TestRefresh
go test ./internal/service/... -v -cover -run TestLogout
go test ./internal/service/... -v -cover -run TestChangePassword

# Password policy
go test ./internal/service/... -v -cover -run TestPasswordPolicy

# Authorization (hasPermission, permissions map)
go test ./internal/service/... -v -cover -run TestHasPermission
go test ./internal/service/... -v -cover -run TestGetUserPermissions
go test ./internal/service/... -v -cover -run TestPermissionsMap
```

### Cobertura en HTML

```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

---

## Tests de Integracion (requiere Docker)

Los tests de integracion usan testcontainers para levantar PostgreSQL 15 y Redis 7
automaticamente. No se requiere ningun servicio externo preexistente.

```bash
go test ./tests/integration/... -v -timeout 300s
```

### Tests de integracion incluidos

| Test | Descripcion |
|---|---|
| `TestIntegration_Login_FullFlow` | Login -> JWT -> Refresh (rotacion) -> Logout -> token revocado |
| `TestIntegration_BruteForce` | 4 intentos fallidos no bloquean; 5to bloquea 15 min |
| `TestIntegration_PermanentLock` | 3 bloqueos en el dia -> bloqueo permanente (locked_until=NULL) |
| `TestIntegration_ChangePassword_History` | Reutilizar ultima 5 passwords -> rechazado; mas antigua -> aceptada |
| `TestIntegration_Bootstrap` | Idempotencia del bootstrap (ON CONFLICT DO NOTHING) |
| `TestIntegration_UserCRUD` | Crear, leer, actualizar, desactivar usuario en BD |
| `TestIntegration_RoleCRUD` | Crear rol, asignar permisos, verificar en BD |
| `TestIntegration_Pagination` | 25 usuarios, page=2, page_size=10 retorna 10 items |
| `TestIntegration_HealthCheck` | PG y Redis responden a ping |
| `TestIntegration_AuditLogs_Immutability` | INSERT funciona; SELECT retorna el registro |
| `TestIntegration_RefreshToken_Cascade` | Borrar usuario borra sus refresh tokens (CASCADE) |

### Variables de entorno para integracion

Los contenedores se crean automaticamente. No se requieren variables de entorno.

---

## Tests de Carga (requiere k6 instalado y servicio corriendo)

Instalar k6: https://k6.io/docs/getting-started/installation/

### Escenario: Login

```bash
k6 run \
  --env BASE_URL=http://localhost:8080 \
  --env APP_KEY=<secret_key> \
  --env TEST_USERNAME=testuser \
  --env TEST_PASSWORD='S3cur3P@ss!' \
  tests/load/scenarios/login_load.js
```

SLA: p95 < 200ms, errores < 1%, 50 VUs sostenidos.

### Escenario: Authz/Verify

```bash
k6 run \
  --env BASE_URL=http://localhost:8080 \
  --env APP_KEY=<secret_key> \
  --env TEST_USERNAME=testuser \
  --env TEST_PASSWORD='S3cur3P@ss!' \
  --env TEST_PERMISSION=inventory.stock.read \
  tests/load/scenarios/authz_verify_load.js
```

SLA: p95 < 50ms, 100 VUs sostenidos por 3 minutos.

### Escenario: Carga Mixta (Recomendado para validar SLA de produccion)

```bash
k6 run \
  --env BASE_URL=http://localhost:8080 \
  --env APP_KEY=<secret_key> \
  --env TEST_USERNAME=testuser \
  --env TEST_PASSWORD='S3cur3P@ss!' \
  tests/load/scenarios/mixed_load.js
```

SLA: p95 global < 200ms, 500 VUs concurrentes por 5 minutos.
Distribucion: 70% authz/verify, 20% login, 10% admin/users.

---

## Criterios de Bloqueo (exit 2)

Estos casos requieren revision obligatoria antes de desplegar a produccion:

1. Cualquier test unitario que falle (`go test ./internal/... -v`)
2. Fallo en `TestIntegration_BruteForce` (seguridad critica)
3. Fallo en `TestIntegration_PermanentLock` (seguridad critica)
4. p95 de login > 200ms en load tests
5. p95 de authz/verify > 50ms en load tests
6. Tasa de errores HTTP > 1% en cualquier escenario de carga

---

## Regresiones Detectadas Durante QA

### REGRESSION: Bug SQL en UserRepository.FindByUsername

**Archivo:** `internal/repository/postgres/user_repository.go`
**Problema:** El campo `userSelectFields` no tenia trailing space, produciendo la query
malformada `...updated_atFROM users...` que PostgreSQL rechaza con `column "id" does not exist`.
**Correccion aplicada:** Agregado espacio al final del string `userSelectFields`.
**Detectado via:** `TestIntegration_Login_FullFlow` y `TestIntegration_BruteForce`.
**Severidad:** CRITICA - ninguna operacion de login/autenticacion funcionaria en produccion.

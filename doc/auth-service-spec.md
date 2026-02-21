# Especificaciones Técnicas: Servicio de Autenticación y Autorización
**Versión:** 1.0.0  
**Fecha:** Febrero 2026  
**Estado:** Borrador  
**Autor:** Equipo de Transformación Digital — Sodexo Chile

---

## 1. Visión General

El **Auth Service** es un microservicio independiente responsable de autenticar usuarios, emitir tokens de acceso y verificar permisos en tiempo real. Actúa como la fuente de verdad central de identidad y autorización para múltiples aplicaciones de la organización.

### 1.1 Objetivos del Servicio

- Autenticar usuarios mediante credenciales y emitir tokens JWT de corta duración acompañados de refresh tokens.
- Gestionar un modelo de autorización híbrido **RBAC + permisos individuales** con soporte para asignaciones temporales.
- Soportar múltiples aplicaciones cliente (web, móvil, escritorio) y múltiples backends consumidores.
- Proveer auditoría completa de eventos de seguridad: quién, qué acción y cuándo.
- Escalar para atender hasta **2.000 usuarios activos** con tiempos de respuesta inferiores a 50 ms en operaciones críticas (login, verificación de token).

### 1.2 Alcance de la Versión 1.0

| Incluido en v1 | Excluido (versiones futuras) |
|---|---|
| Autenticación usuario/contraseña | SSO / OAuth2 externo |
| JWT + Refresh Tokens | Revocación inmediata de access tokens |
| RBAC + permisos individuales | MFA / 2FA |
| Roles temporales con expiración | Federación de identidad |
| Multi-aplicación | API Keys para integraciones B2B |
| Auditoría de eventos | |
| Bootstrap de admin inicial | |

---

## 2. Arquitectura

### 2.1 Stack Tecnológico

| Componente | Tecnología |
|---|---|
| Lenguaje | Go 1.22+ |
| Framework HTTP | Fiber v2 o Chi |
| Base de datos principal | PostgreSQL 15+ |
| Caché / Refresh Tokens | Redis 7+ |
| Hashing de contraseñas | bcrypt (costo ≥ 12) |
| Tokens | JWT (RS256 — clave asimétrica) |
| Configuración | Variables de entorno + archivo YAML |
| Contenerización | Docker + Docker Compose |
| Despliegue destino | Azure (Container Apps / AKS) |

> **Nota sobre RS256:** Se elige firma asimétrica (clave privada para firmar, clave pública para verificar) para que los servicios consumidores (.NET Core, Python, Go) puedan verificar tokens localmente sin necesidad de llamar al Auth Service en cada request, reduciendo latencia.

### 2.2 Diagrama de Componentes

```
┌────────────────────────────────────────────────────────────────┐
│                        Clientes                                │
│  React Web  │  App Móvil  │  App Escritorio                   │
└──────────────────────────┬─────────────────────────────────────┘
                           │ HTTPS
┌──────────────────────────▼─────────────────────────────────────┐
│                      Auth Service (Go)                         │
│                                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐ │
│  │  Auth API    │  │  Authz API   │  │   Admin API          │ │
│  │  /auth/*     │  │  /verify     │  │   /admin/*           │ │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘ │
│         └─────────────────┴──────────────────────┘            │
│                           │                                    │
│              ┌────────────▼────────────┐                       │
│              │     Core Domain         │                       │
│              │  Auth · Authz · Audit   │                       │
│              └──────┬──────────────────┘                       │
└─────────────────────┼──────────────────────────────────────────┘
                      │
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
   PostgreSQL       Redis       Log Storage
   (datos)      (ref. tokens   (auditoría)
                 / caché)

┌──────────────────────────────────────────────────────┐
│              Servicios Consumidores Backend           │
│   .NET Core App  │  Python Service  │  Go Service    │
│   (verifican JWT con clave pública — sin llamada)    │
└──────────────────────────────────────────────────────┘
```

---

## 3. Modelo de Datos

### 3.1 Entidades Principales

#### `applications`
Representa cada sistema o aplicación registrada en el Auth Service.

```sql
CREATE TABLE applications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL UNIQUE,
    slug        VARCHAR(50)  NOT NULL UNIQUE,   -- ej: 'hospitality-app'
    secret_key  VARCHAR(255) NOT NULL,           -- para autenticar al app en /verify
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### `users`
```sql
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(100) NOT NULL UNIQUE,
    email           VARCHAR(255) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    must_change_pwd BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMPTZ,
    failed_attempts INT NOT NULL DEFAULT 0,
    locked_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### `permissions`
Conjunto finito de permisos definidos por cada aplicación. Se registran en la BD, no en código.

```sql
CREATE TABLE permissions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id),
    code           VARCHAR(100) NOT NULL,   -- ej: 'inventory.stock.read'
    description    TEXT,
    scope_type     VARCHAR(20) NOT NULL,    -- 'global' | 'module' | 'resource' | 'action'
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, code)
);
```

> **Convención de códigos de permiso:** `{módulo}.{recurso}.{acción}`  
> Ejemplos: `inventory.stock.read`, `finance.ceco.write`, `reports.monthly.export`

#### `cost_centers` (CeCo)
Recurso organizacional para controlar el acceso a datos de centros de costo específicos.

```sql
CREATE TABLE cost_centers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id),
    code           VARCHAR(50) NOT NULL,
    name           VARCHAR(150) NOT NULL,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, code)
);
```

#### `roles`
Agrupadores de permisos. Son dinámicos y se crean desde la aplicación.

```sql
CREATE TABLE roles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id),
    name           VARCHAR(100) NOT NULL,
    description    TEXT,
    is_system      BOOLEAN NOT NULL DEFAULT FALSE,  -- TRUE solo para 'admin' bootstrap
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, name)
);
```

#### `role_permissions`
```sql
CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);
```

#### `user_roles`
Asignación de roles a usuarios, con soporte de vigencia temporal.

```sql
CREATE TABLE user_roles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id        UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    application_id UUID NOT NULL REFERENCES applications(id),
    granted_by     UUID NOT NULL REFERENCES users(id),
    valid_from     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_until    TIMESTAMPTZ,                      -- NULL = sin expiración
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### `user_permissions`
Permisos individuales adicionales, por fuera del rol. También con vigencia temporal.

```sql
CREATE TABLE user_permissions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id  UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    application_id UUID NOT NULL REFERENCES applications(id),
    granted_by     UUID NOT NULL REFERENCES users(id),
    valid_from     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_until    TIMESTAMPTZ,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### `user_cost_centers`
Control de acceso a datos por CeCo.

```sql
CREATE TABLE user_cost_centers (
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cost_center_id UUID NOT NULL REFERENCES cost_centers(id) ON DELETE CASCADE,
    application_id UUID NOT NULL REFERENCES applications(id),
    granted_by     UUID NOT NULL REFERENCES users(id),
    valid_from     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_until    TIMESTAMPTZ,
    PRIMARY KEY (user_id, cost_center_id)
);
```

#### `refresh_tokens`
```sql
CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_id      UUID NOT NULL REFERENCES applications(id),
    token_hash  VARCHAR(255) NOT NULL UNIQUE,
    device_info JSONB,                            -- user-agent, IP, tipo de cliente
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    is_revoked  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### `audit_logs`
```sql
CREATE TABLE audit_logs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     VARCHAR(50) NOT NULL,         -- ver sección 6
    application_id UUID REFERENCES applications(id),
    user_id        UUID REFERENCES users(id),
    actor_id       UUID REFERENCES users(id),    -- quién realizó la acción
    resource_type  VARCHAR(50),
    resource_id    UUID,
    old_value      JSONB,
    new_value      JSONB,
    ip_address     INET,
    user_agent     TEXT,
    success        BOOLEAN NOT NULL,
    error_message  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Índices para consultas de auditoría
CREATE INDEX idx_audit_user_id     ON audit_logs (user_id, created_at DESC);
CREATE INDEX idx_audit_actor_id    ON audit_logs (actor_id, created_at DESC);
CREATE INDEX idx_audit_event_type  ON audit_logs (event_type, created_at DESC);
CREATE INDEX idx_audit_app_id      ON audit_logs (application_id, created_at DESC);
```

---

## 4. API REST

### 4.1 Convenciones

- **Base URL:** `https://auth.internal.sodexo.cl/api/v1`
- **Formato:** JSON (`Content-Type: application/json`)
- **Autenticación de aplicación:** Header `X-App-Key: <secret_key>` en todos los endpoints excepto `/health`
- **Autenticación de usuario:** Header `Authorization: Bearer <access_token>`
- **Errores:** Formato estándar

```json
{
  "error": {
    "code": "INVALID_CREDENTIALS",
    "message": "Usuario o contraseña incorrectos",
    "details": null
  }
}
```

---

### 4.2 Endpoints de Autenticación (`/auth`)

#### `POST /auth/login`
Autentica al usuario y retorna tokens.

**Request:**
```json
{
  "username": "jperez",
  "password": "••••••••"
}
```

**Response 200:**
```json
{
  "access_token": "eyJhbGci...",
  "refresh_token": "dGhpcyBp...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "user": {
    "id": "uuid",
    "username": "jperez",
    "email": "jperez@sodexo.com",
    "must_change_password": false
  }
}
```

**Errores posibles:** `INVALID_CREDENTIALS`, `ACCOUNT_LOCKED`, `ACCOUNT_INACTIVE`, `APPLICATION_NOT_FOUND`

---

#### `POST /auth/refresh`
Renueva el access token usando el refresh token.

**Request:**
```json
{
  "refresh_token": "dGhpcyBp..."
}
```

**Response 200:** Igual que `/auth/login` (sin `user` completo, solo nuevos tokens)

**Errores posibles:** `TOKEN_INVALID`, `TOKEN_EXPIRED`, `TOKEN_REVOKED`

---

#### `POST /auth/logout`
Invalida el refresh token activo.  
**Headers:** `Authorization: Bearer <access_token>`

**Response 204:** Sin cuerpo.

---

#### `POST /auth/change-password`
**Headers:** `Authorization: Bearer <access_token>`

**Request:**
```json
{
  "current_password": "••••••••",
  "new_password": "••••••••"
}
```

**Response 204:** Sin cuerpo.

---

### 4.3 Endpoints de Autorización (`/authz`)

#### `POST /authz/verify`
Verifica si el usuario tiene un permiso específico. Usado por servicios backend.

**Headers:** `Authorization: Bearer <access_token>`, `X-App-Key: <secret>`

**Request:**
```json
{
  "permission": "inventory.stock.write",
  "cost_center_id": "uuid-del-ceco"    // opcional
}
```

**Response 200:**
```json
{
  "allowed": true,
  "user_id": "uuid",
  "username": "jperez",
  "permission": "inventory.stock.write",
  "evaluated_at": "2026-02-18T10:30:00Z"
}
```

> Este endpoint está diseñado para ser llamado por los servicios backend cuando no puedan o no quieran validar el JWT localmente.

---

#### `GET /authz/me/permissions`
Retorna todos los permisos efectivos del usuario autenticado para la aplicación actual.  
**Headers:** `Authorization: Bearer <access_token>`

**Response 200:**
```json
{
  "user_id": "uuid",
  "application": "hospitality-app",
  "roles": ["chef", "bodeguero-temporal"],
  "permissions": [
    "inventory.stock.read",
    "inventory.stock.write",
    "reports.monthly.export"
  ],
  "cost_centers": ["CC001", "CC002"],
  "temporary_roles": [
    {
      "role": "bodeguero-temporal",
      "valid_until": "2026-03-01T00:00:00Z"
    }
  ]
}
```

---

### 4.4 Endpoints de Administración (`/admin`)

Todos requieren el permiso `admin.system.manage`.

#### Gestión de Roles

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/admin/roles` | Listar roles de la aplicación |
| `POST` | `/admin/roles` | Crear nuevo rol |
| `GET` | `/admin/roles/:id` | Detalle de un rol |
| `PUT` | `/admin/roles/:id` | Actualizar rol |
| `DELETE` | `/admin/roles/:id` | Desactivar rol |
| `POST` | `/admin/roles/:id/permissions` | Asignar permisos a rol |
| `DELETE` | `/admin/roles/:id/permissions/:pid` | Remover permiso de rol |

#### Gestión de Usuarios

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/admin/users` | Listar usuarios |
| `POST` | `/admin/users` | Crear usuario |
| `GET` | `/admin/users/:id` | Detalle de usuario |
| `PUT` | `/admin/users/:id` | Actualizar usuario |
| `POST` | `/admin/users/:id/roles` | Asignar rol (con `valid_until` opcional) |
| `DELETE` | `/admin/users/:id/roles/:rid` | Revocar rol |
| `POST` | `/admin/users/:id/permissions` | Asignar permiso individual |
| `DELETE` | `/admin/users/:id/permissions/:pid` | Revocar permiso individual |
| `POST` | `/admin/users/:id/cost-centers` | Asignar CeCos |
| `POST` | `/admin/users/:id/unlock` | Desbloquear cuenta |
| `POST` | `/admin/users/:id/reset-password` | Forzar reset de contraseña |

#### Gestión de Permisos

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/admin/permissions` | Listar permisos disponibles |
| `POST` | `/admin/permissions` | Registrar nuevo permiso |
| `DELETE` | `/admin/permissions/:id` | Eliminar permiso |

#### Gestión de CeCos

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/admin/cost-centers` | Listar CeCos |
| `POST` | `/admin/cost-centers` | Crear CeCo |
| `PUT` | `/admin/cost-centers/:id` | Actualizar CeCo |

#### Auditoría

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/admin/audit-logs` | Consultar logs (con filtros) |

**Filtros disponibles para `/admin/audit-logs`:**  
`user_id`, `actor_id`, `event_type`, `from_date`, `to_date`, `application_id`, `success`

---

### 4.5 Endpoints de Utilidad

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/health` | Health check del servicio |
| `GET` | `/.well-known/jwks.json` | Clave pública RSA para verificar JWTs (para consumidores) |

---

## 5. Diseño del JWT

### 5.1 Access Token
Firmado con **RS256**. Vida útil: **60 minutos**.

**Header:**
```json
{
  "alg": "RS256",
  "typ": "JWT",
  "kid": "2026-02-key-01"
}
```

**Payload:**
```json
{
  "sub": "uuid-del-usuario",
  "username": "jperez",
  "email": "jperez@sodexo.com",
  "app": "hospitality-app",
  "roles": ["chef", "bodeguero-temporal"],
  "iat": 1708250400,
  "exp": 1708254000,
  "jti": "uuid-único-del-token"
}
```

> **Nota de diseño — JWT mínimo:** El token lleva únicamente los **roles** del usuario. Los `extra_permissions` (permisos individuales puntuales) y los `cost_centers` autorizados **no se incluyen en el token**: residen exclusivamente en el mapa de permisos del Auth Service. Los backends obtienen el contexto completo del usuario (roles efectivos + extra_permissions + cost_centers) llamando a `GET /authz/me/permissions` una sola vez al inicio de la sesión, cacheándolo en memoria usando el `jti` como clave con el mismo TTL que la vida del access token (60 min). Cada request siguiente es verificado completamente en memoria, sin ninguna llamada de red adicional.

---

### 5.2 Mapa de Permisos para Verificación Local

El Auth Service publica un endpoint que los backends consumen una sola vez y cachean con TTL. El mapa contiene la relación completa de permisos, qué roles los otorgan, qué permisos adicionales existen y qué CeCos están definidos.

#### `GET /authz/permissions-map`

**Headers:** `X-App-Key: <secret>`

**Response 200:**
```json
{
  "application": "hospitality-app",
  "generated_at": "2026-02-18T10:00:00Z",
  "version": "a3f8c21d",
  "permissions": {
    "inventory.stock.read":   { "roles": ["chef", "bodeguero", "admin"], "description": "Ver stock" },
    "inventory.stock.write":  { "roles": ["bodeguero", "admin"],         "description": "Modificar stock" },
    "reports.monthly.export": { "roles": ["supervisor", "admin"],        "description": "Exportar reporte mensual" },
    "reports.special.export": { "roles": ["admin"],                      "description": "Exportar reporte especial" }
  },
  "cost_centers": {
    "CC001": { "code": "CC001", "name": "Casino Central",    "is_active": true },
    "CC002": { "code": "CC002", "name": "Comedor Ejecutivo", "is_active": true }
  },
  "signature": "base64url(RSA-SHA256(payload))"
}
```

El campo `version` es un hash del estado actual del mapa. Los backends pueden usarlo para detectar si el mapa cambió sin necesidad de descargar el payload completo (ver endpoint de polling abajo).

#### Firma del Mapa (Integridad y Autenticidad)

El campo `signature` es una firma **RSA-SHA256** generada por el Auth Service usando la **misma clave privada** que firma los JWTs. Esto garantiza que:

- Solo el Auth Service —poseedor de la clave privada— puede emitir un mapa válido.
- Cualquier modificación del contenido en tránsito o en el caché local invalida la firma.
- Los backends verifican con la **clave pública ya cacheada** desde `/.well-known/jwks.json`, sin infraestructura adicional.

**¿Qué se firma?** El contenido firmado es el objeto `payload` canónico: los campos `application`, `generated_at`, `version`, `permissions` y `cost_centers` serializados en JSON con claves ordenadas lexicográficamente y sin espacios en blanco.

```
payload_to_sign = canonicalJSON({
  application, generated_at, version, permissions, cost_centers
})

signature = base64url( RSA-SHA256( payload_to_sign, privateKey ) )
```

**Verificación en el backend al recibir el mapa:**
```
1. Reconstruir el payload canónico (mismos campos, mismo orden, sin el campo signature).
2. Verificar: RSA-SHA256-Verify( payload_canónico, base64url_decode(signature), publicKey ).
3. Si la verificación falla → descartar el mapa, loguear el error, conservar el caché anterior.
4. Si el caché anterior también es inválido → denegar todos los accesos y alertar.
```

**Verificación de referencia en Go:**
```go
import (
    "crypto"
    "crypto/rsa"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
)

func VerifyPermissionsMap(raw []byte, pubKey *rsa.PublicKey) error {
    var mapResp PermissionsMapResponse
    if err := json.Unmarshal(raw, &mapResp); err != nil {
        return err
    }

    // Reconstruir payload canónico sin el campo signature
    canonical, err := canonicalJSON(mapResp.Payload())
    if err != nil {
        return err
    }

    digest := sha256.Sum256(canonical)

    sigBytes, err := base64.RawURLEncoding.DecodeString(mapResp.Signature)
    if err != nil {
        return err
    }

    return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, digest[:], sigBytes)
}
```

> **Nota:** La rotación de claves RSA (sección 7.4) invalida automáticamente los mapas firmados con la clave anterior. Durante el período de transición, el Auth Service firma con la clave nueva; los backends que aún tengan la clave antigua en caché simplemente fallarán la verificación y descargarán tanto la clave pública actualizada como el mapa nuevo.

---

#### `GET /authz/permissions-map/version`

Endpoint liviano para que los backends verifiquen periódicamente si el mapa cambió.

**Response 200:**
```json
{
  "application": "hospitality-app",
  "version": "a3f8c21d",
  "generated_at": "2026-02-18T10:00:00Z"
}
```

Si `version` difiere del valor cacheado, el backend descarga el mapa completo y **verifica la firma** antes de reemplazar el caché.

---

### 5.3 Lógica de Verificación Local en los Backends

Con el mapa global cacheado y el contexto de usuario cacheado por `jti`, la verificación de un permiso es completamente en memoria.

El backend mantiene **dos cachés distintos**:

| Caché | Clave | Fuente | TTL |
|---|---|---|---|
| Mapa global de permisos | `app_slug` | `GET /authz/permissions-map` | 5 min (con polling de versión) |
| Contexto por usuario | `jti` del token | `GET /authz/me/permissions` | 60 min (vida del access token) |

```
Algoritmo HasPermission(jwt, userContext, permissionCode, costCenterCode?):

1. Verificar firma y expiración del JWT (clave pública RSA cacheada).
2. Obtener userContext desde caché por jwt.jti.
   Si no existe → llamar GET /authz/me/permissions y cachear.
3. Si permissionCode está en userContext.extra_permissions → paso 5.
4. Buscar en el mapa global: rolesConPermiso = permissionsMap[permissionCode].roles
   Si ninguno de jwt.roles está en rolesConPermiso → DENEGADO.
5. Si se especificó costCenterCode:
   Si costCenterCode NO está en userContext.cost_centers → DENEGADO.
6. → PERMITIDO.
```

**Implementación de referencia en Go:**
```go
func HasPermission(
    claims     JWTClaims,
    userCtx    UserContext,   // obtenido de /authz/me/permissions, cacheado por jti
    permMap    PermissionsMap,
    required   string,
    costCenter string,
) bool {
    // 1. Override individual del usuario
    if slices.Contains(userCtx.ExtraPermissions, required) {
        if costCenter != "" {
            return slices.Contains(userCtx.CostCenters, costCenter)
        }
        return true
    }

    // 2. Verificar vía roles en el mapa global
    entry, exists := permMap.Permissions[required]
    if !exists {
        return false
    }
    for _, role := range claims.Roles {
        if slices.Contains(entry.Roles, role) {
            if costCenter != "" {
                return slices.Contains(userCtx.CostCenters, costCenter)
            }
            return true
        }
    }
    return false
}
```

**Implementación de referencia en Python:**
```python
def has_permission(
    claims: dict,
    user_ctx: dict,    # obtenido de /authz/me/permissions, cacheado por jti
    perm_map: dict,
    required: str,
    cost_center: str = None,
) -> bool:
    # 1. Override individual del usuario
    if required in user_ctx.get("extra_permissions", []):
        if cost_center:
            return cost_center in user_ctx.get("cost_centers", [])
        return True

    # 2. Verificar vía roles en el mapa global
    entry = perm_map["permissions"].get(required)
    if not entry:
        return False
    if any(role in entry["roles"] for role in claims.get("roles", [])):
        if cost_center:
            return cost_center in user_ctx.get("cost_centers", [])
        return True
    return False
```

**Implementación de referencia en C# (.NET Core):**
```csharp
public bool HasPermission(
    JwtClaims    claims,
    UserContext  userCtx,    // obtenido de /authz/me/permissions, cacheado por jti
    PermissionsMap permMap,
    string       required,
    string?      costCenter = null)
{
    // 1. Override individual del usuario
    if (userCtx.ExtraPermissions.Contains(required))
        return costCenter == null || userCtx.CostCenters.Contains(costCenter);

    // 2. Verificar vía roles en el mapa global
    if (!permMap.Permissions.TryGetValue(required, out var entry))
        return false;

    if (claims.Roles.Any(r => entry.Roles.Contains(r)))
        return costCenter == null || userCtx.CostCenters.Contains(costCenter);

    return false;
}
```

**Implementación de referencia en JavaScript (Node.js / Web):**
```javascript
/**
 * @param {Object}      claims     - Payload decodificado del JWT
 * @param {Object}      userCtx    - Contexto del usuario cacheado por jti
 *                                   (obtenido de /authz/me/permissions)
 * @param {Object}      permMap    - Mapa global cacheado desde /authz/permissions-map
 * @param {string}      required   - Permiso requerido, ej: 'inventory.stock.write'
 * @param {string|null} costCenter - CeCo requerido, ej: 'CC001' (opcional)
 * @returns {boolean}
 */
function hasPermission(claims, userCtx, permMap, required, costCenter = null) {
  const extraPermissions = userCtx.extra_permissions ?? [];
  const costCenters      = userCtx.cost_centers     ?? [];
  const roles            = claims.roles             ?? [];

  // 1. Override individual del usuario
  if (extraPermissions.includes(required)) {
    return costCenter === null || costCenters.includes(costCenter);
  }

  // 2. Verificar vía roles en el mapa global
  const entry = permMap.permissions?.[required];
  if (!entry) return false;

  const hasRole = roles.some(role => entry.roles.includes(role));
  if (!hasRole) return false;

  // 3. Validar CeCo si aplica
  return costCenter === null || costCenters.includes(costCenter);
}

// Ejemplo de uso
// const allowed = hasPermission(jwtClaims, userContext, cachedPermMap, 'inventory.stock.write', 'CC001');
```

---

### 5.4 Estrategia de Caché del Mapa en los Backends

| Parámetro | Valor recomendado |
|---|---|
| TTL del mapa de permisos | 5 minutos |
| Polling de versión | Cada 2 minutos (liviano) |
| TTL de la clave pública RSA | 60 minutos |
| Recarga forzada ante `version` distinta | Inmediata |
| Almacenamiento | En memoria del proceso (no requiere Redis en el backend) |

El Auth Service invalida el `version` hash cada vez que se modifica cualquier permiso, rol o asignación de CeCo, lo que garantiza que los backends adopten los cambios dentro del TTL de polling.

### 5.5 Refresh Token
- Generado como un UUID v4 aleatorio, almacenado como **hash bcrypt** en Redis y PostgreSQL.
- Vida útil configurable: **7 días** (web), **30 días** (móvil/escritorio).
- Rotación automática: cada uso genera un nuevo refresh token e invalida el anterior.
- Almacenamiento en Redis: `refresh:<token_hash>` con TTL.

---

## 6. Sistema de Auditoría

### 6.1 Tipos de Eventos

| Categoría | Código de Evento |
|---|---|
| Autenticación | `AUTH_LOGIN_SUCCESS`, `AUTH_LOGIN_FAILED`, `AUTH_LOGOUT`, `AUTH_TOKEN_REFRESHED`, `AUTH_PASSWORD_CHANGED`, `AUTH_PASSWORD_RESET`, `AUTH_ACCOUNT_LOCKED` |
| Autorización | `AUTHZ_PERMISSION_GRANTED`, `AUTHZ_PERMISSION_DENIED` |
| Gestión de usuarios | `USER_CREATED`, `USER_UPDATED`, `USER_DEACTIVATED`, `USER_UNLOCKED` |
| Gestión de roles | `ROLE_CREATED`, `ROLE_UPDATED`, `ROLE_DELETED`, `ROLE_PERMISSION_ASSIGNED`, `ROLE_PERMISSION_REVOKED` |
| Asignaciones | `USER_ROLE_ASSIGNED`, `USER_ROLE_REVOKED`, `USER_PERMISSION_ASSIGNED`, `USER_PERMISSION_REVOKED`, `USER_COST_CENTER_ASSIGNED` |

### 6.2 Esquema del Log

Cada registro debe incluir obligatoriamente:
- **Quién** (`actor_id`, `user_id`, `ip_address`, `user_agent`)
- **Qué** (`event_type`, `resource_type`, `resource_id`, `old_value`, `new_value`)
- **Cuándo** (`created_at` en UTC con zona horaria)
- **Resultado** (`success`, `error_message`)

Los logs son **inmutables**: no se permiten UPDATE ni DELETE sobre `audit_logs`.

---

## 7. Seguridad

### 7.1 Contraseñas
- Hashing con **bcrypt**, costo mínimo 12.
- Política de contraseñas: mínimo 10 caracteres, al menos 1 mayúscula, 1 número, 1 símbolo.
- Historial de últimas 5 contraseñas (no reutilizar).

### 7.2 Protección contra Fuerza Bruta
- Máximo **5 intentos fallidos** consecutivos → cuenta bloqueada por **15 minutos**.
- Después de 3 bloqueos en el mismo día → bloqueo manual que requiere desbloqueo por admin.
- Registro de todos los intentos en auditoría.

### 7.3 Comunicación
- Todo el tráfico sobre **HTTPS/TLS 1.2+**.
- Headers de seguridad obligatorios en todas las respuestas:
  - `Strict-Transport-Security`
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`

### 7.4 Rotación de Claves RSA
- Las claves RSA se almacenan como secretos en Azure Key Vault.
- El endpoint `/.well-known/jwks.json` expone las claves públicas con su `kid`.
- La rotación se realiza sin downtime: durante un período de transición, ambas claves (anterior y nueva) son válidas.

---

## 8. Bootstrap del Sistema

Al iniciarse por primera vez con una BD vacía, el servicio debe:

1. Detectar que no existen aplicaciones ni usuarios.
2. Crear la aplicación `system` y el rol `admin` con todos los permisos de gestión.
3. Crear el usuario **administrador inicial** con credenciales tomadas de variables de entorno (`BOOTSTRAP_ADMIN_USER`, `BOOTSTRAP_ADMIN_PASSWORD`).
4. Forzar cambio de contraseña en el primer login (`must_change_pwd = true`).
5. Registrar el evento `SYSTEM_BOOTSTRAP` en auditoría.
6. Una vez ejecutado el bootstrap, no volver a ejecutarse aunque se reinicie el servicio.

---

## 9. Integración para Consumidores

### 9.1 Flujo Completo de Verificación Local (Recomendado)

Este es el flujo estándar para los tres backends. Una vez completado el arranque, ningún request de verificación requiere llamadas al Auth Service.

```
Arranque del backend:
  1. Descargar clave pública RSA desde /.well-known/jwks.json   (cachear 60 min)
  2. Descargar mapa global desde /authz/permissions-map          (cachear 5 min)
  3. Iniciar polling liviano a /authz/permissions-map/version    (cada 2 min)
     → si version cambia, recargar el mapa global completo

Por cada request del cliente (primera vez con ese token):
  4. Extraer Bearer token del header Authorization
  5. Verificar firma RS256 y expiración del JWT con clave pública cacheada
  6. Buscar contexto de usuario en caché por jwt.jti
     → Si no existe: llamar GET /authz/me/permissions y cachear con TTL = 60 min
  7. Evaluar HasPermission(claims, userContext, permMap, required, costCenter?)
     → Todo en memoria, sin llamada de red

Por cada request del cliente (token ya visto):
  4. Extraer y verificar JWT localmente
  5. userContext ya está en caché → verificación 100% en memoria
```

### 9.2 Verificación Delegada (Alternativa)

Para casos puntuales donde la verificación local no sea posible (por ejemplo, scripts o herramientas de automatización), los backends pueden llamar a `POST /authz/verify` enviando el access token y el permiso a verificar. No se recomienda como flujo principal por la latencia adicional.

### 9.3 Headers Estándar para Backends

Al validar el JWT, los backends deben propagar los siguientes headers hacia los servicios internos:

```
X-User-Id:      <uuid>
X-Username:     <username>
X-App:          <app-slug>
X-Roles:        <rol1,rol2>
X-Cost-Centers: <CC001,CC002>
```

---

## 10. Configuración del Servicio

```yaml
# config.yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  graceful_shutdown_timeout: 15s

database:
  host: ${DB_HOST}
  port: 5432
  name: ${DB_NAME}
  user: ${DB_USER}
  password: ${DB_PASSWORD}
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: 5m

redis:
  addr: ${REDIS_ADDR}
  password: ${REDIS_PASSWORD}
  db: 0

jwt:
  private_key_path: ${JWT_PRIVATE_KEY_PATH}   # o Azure Key Vault ref
  public_key_path: ${JWT_PUBLIC_KEY_PATH}
  access_token_ttl: 60m
  refresh_token_ttl_web: 168h      # 7 días
  refresh_token_ttl_mobile: 720h   # 30 días

security:
  max_failed_attempts: 5
  lockout_duration: 15m
  bcrypt_cost: 12
  password_history: 5

bootstrap:
  admin_user: ${BOOTSTRAP_ADMIN_USER}
  admin_password: ${BOOTSTRAP_ADMIN_PASSWORD}

logging:
  level: info
  format: json
```

---

## 11. Estructura del Proyecto (Go)

```
auth-service/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── config/
│   ├── domain/
│   │   ├── user.go
│   │   ├── role.go
│   │   ├── permission.go
│   │   ├── cost_center.go
│   │   └── audit.go
│   ├── repository/
│   │   ├── postgres/
│   │   └── redis/
│   ├── service/
│   │   ├── auth_service.go
│   │   ├── authz_service.go
│   │   ├── user_service.go
│   │   ├── role_service.go
│   │   └── audit_service.go
│   ├── handler/
│   │   ├── auth_handler.go
│   │   ├── authz_handler.go
│   │   └── admin_handler.go
│   ├── middleware/
│   │   ├── jwt_middleware.go
│   │   ├── app_key_middleware.go
│   │   ├── permission_middleware.go
│   │   └── audit_middleware.go
│   ├── token/
│   │   └── jwt.go
│   └── bootstrap/
│       └── initializer.go
├── migrations/
│   ├── 001_initial_schema.sql
│   └── 002_seed_permissions.sql
├── docker-compose.yml
├── Dockerfile
├── config.yaml
└── README.md
```

---

## 12. Rendimiento y SLAs

| Métrica | Objetivo |
|---|---|
| Latencia `POST /auth/login` | < 200 ms (p95) |
| Latencia `POST /authz/verify` | < 50 ms (p95) |
| Latencia verificación JWT local | < 5 ms (operación en memoria) |
| Usuarios concurrentes soportados | 500+ |
| Usuarios totales | 2.000 |
| Disponibilidad | 99.5% mensual |
| Tiempo de recuperación (RTO) | < 5 minutos |

**Estrategias de optimización:**
- Pool de conexiones a PostgreSQL (pgx).
- Permisos efectivos del usuario cacheados en Redis con TTL de 5 minutos. Se invalida el caché cuando se modifica el rol o permiso del usuario.
- Índices en todas las claves foráneas y campos de consulta frecuente.
- Verificación JWT local en los backends para eliminar round-trips innecesarios.

---

## 13. Plan de Implementación

### Fase 1 — Core (Semanas 1–2)
- Configuración del proyecto Go, Docker Compose (PostgreSQL + Redis).
- Migraciones de BD.
- Bootstrap del sistema (`admin` inicial).
- Endpoints de autenticación: `login`, `refresh`, `logout`.
- Generación y validación de JWT RS256.
- Endpoint `/.well-known/jwks.json`.

### Fase 2 — Autorización (Semanas 3–4)
- Motor RBAC + permisos individuales.
- Soporte de vigencia temporal en roles y permisos.
- Control de acceso por CeCo.
- Endpoint `POST /authz/verify` y `GET /authz/me/permissions`.

### Fase 3 — Administración y Auditoría (Semanas 5–6)
- API de administración completa (roles, usuarios, permisos, CeCos).
- Sistema de auditoría (logs inmutables).
- Protección contra fuerza bruta y bloqueo de cuentas.

### Fase 4 — Calidad y Hardening (Semana 7–8)
- Tests de integración (cobertura ≥ 80%).
- Documentación OpenAPI / Swagger.
- Load testing (Grafana k6 o hey).
- Revisión de seguridad.
- Despliegue en Azure (entorno de staging).

---

## 14. Decisiones de Diseño y Justificaciones

| Decisión | Alternativa Considerada | Razón de la Elección |
|---|---|---|
| RS256 para JWT | HS256 | RS256 permite verificación local en múltiples backends sin compartir secreto |
| Roles + extra_permissions en JWT (sin lista de permisos) | Todos los permisos en el token | Con >50 permisos, el JWT crecería 2–3 KB por request. Los backends resuelven permisos localmente con el mapa cacheado |
| Mapa de permisos cacheado en los backends | Bitset de permisos en el token | El mapa es legible, fácil de implementar en Go/Python/.NET y cambia raramente; el bitset sería más compacto pero más complejo de mantener en tres lenguajes |
| CeCos validados desde el JWT + mapa | Solo en base de datos | Permite validación sin llamada de red; el mapa mantiene CeCos actualizados con TTL de polling |
| PostgreSQL para datos maestros | MySQL | Soporte nativo de JSONB, UUID, INET, arrays; mejor para datos de auditoría |
| Redis para refresh tokens | Solo PostgreSQL | TTL nativo, O(1) para lookup, reduce carga en PG en operaciones frecuentes |
| Logs inmutables en PostgreSQL | Sistema externo (ELK) | Reduce dependencias en v1; se puede federar a ELK en versiones futuras |
| Firma RSA del mapa de permisos | HMAC-SHA256 con secreto compartido / Hash simple | RSA reutiliza la infraestructura existente: los backends ya tienen la clave pública cacheada. Garantiza autenticidad (solo el Auth Service puede firmar) e integridad sin distribuir secretos adicionales |
| Polling de versión del mapa cada 2 min | TTL fijo sin polling | Permite propagación rápida de cambios sin descargar el mapa completo en cada ciclo |

---

*Fin del documento de especificaciones v1.0.0*

// Package domain contiene los tipos de datos centrales del sistema Sentinel.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken representa un token de refresco almacenado en PostgreSQL.
// El flujo de refresh token en Sentinel es el siguiente:
//  1. Al hacer login exitoso, se genera un UUID v4 aleatorio (el token raw).
//  2. El UUID raw se envía al cliente y también se usa como clave en Redis.
//  3. Se genera un hash bcrypt del UUID raw; ese hash se almacena aquí (TokenHash)
//     y también en Redis como valor asociado a la clave.
//  4. Al refrescar, se verifica que el UUID raw coincida con el hash almacenado.
//
// Este diseño garantiza que, incluso si la base de datos es comprometida,
// los tokens raw no pueden ser reconstituidos desde los hashes.
type RefreshToken struct {
	// ID es el identificador único del registro de refresh token (UUID v4).
	ID uuid.UUID

	// UserID referencia el usuario propietario del token.
	UserID uuid.UUID

	// AppID referencia la aplicación cliente para la que se emitió el token.
	AppID uuid.UUID

	// TokenHash es el hash bcrypt del UUID raw del refresh token.
	// Nunca se almacena el token raw; solo su hash para verificación posterior.
	TokenHash string

	// DeviceInfo contiene información sobre el dispositivo y cliente que
	// realizó el login. Se almacena en la columna device_info (tipo JSONB) de PostgreSQL.
	DeviceInfo DeviceInfo

	// ExpiresAt es la fecha y hora de expiración del token.
	// Para clientes web: 7 días. Para móvil/desktop: 30 días.
	// Un token expirado es rechazado aunque no haya sido revocado.
	ExpiresAt time.Time

	// UsedAt es la fecha y hora en que el token fue usado por última vez para refrescar.
	// Es nil si el token aún no ha sido usado para obtener un nuevo access token.
	UsedAt *time.Time

	// IsRevoked indica si el token fue invalidado explícitamente (por logout o rotación).
	// Un token revocado es rechazado aunque no haya expirado.
	IsRevoked bool

	// CreatedAt es la fecha y hora de creación del registro.
	CreatedAt time.Time
}

// DeviceInfo almacena el contexto del cliente que realizó el login.
// Se persiste como JSONB en la columna device_info de la tabla refresh_tokens,
// y permite auditar desde qué dispositivo y tipo de cliente se originó cada sesión.
type DeviceInfo struct {
	// UserAgent es el valor del header User-Agent del cliente HTTP al momento del login.
	// Permite identificar el navegador, sistema operativo o app móvil.
	UserAgent string `json:"user_agent"`

	// IP es la dirección IP del cliente al momento del login.
	// Se obtiene del header X-Forwarded-For o de la conexión directa.
	IP string `json:"ip"`

	// ClientType es el tipo de cliente declarado explícitamente en el cuerpo del login.
	// Determina el TTL del refresh token: "web" = 7 días, "mobile"/"desktop" = 30 días.
	// Nunca se infiere del User-Agent; el cliente debe declararlo explícitamente.
	ClientType string `json:"client_type"`
}

// ClientType representa el tipo de cliente que realiza el login.
// Es un campo obligatorio del cuerpo de la solicitud POST /auth/login.
// Su valor afecta directamente el tiempo de vida del refresh token emitido.
type ClientType string

const (
	// ClientTypeWeb indica que el cliente es un navegador web.
	// El refresh token tendrá una duración de 7 días (168 horas).
	ClientTypeWeb ClientType = "web"

	// ClientTypeMobile indica que el cliente es una aplicación móvil (iOS/Android).
	// El refresh token tendrá una duración de 30 días (720 horas).
	ClientTypeMobile ClientType = "mobile"

	// ClientTypeDesktop indica que el cliente es una aplicación de escritorio.
	// El refresh token tendrá una duración de 30 días (720 horas), igual que mobile.
	ClientTypeDesktop ClientType = "desktop"
)

// IsValidClientType informa si el valor ct es un ClientType admitido por el sistema.
// Se usa para validar el campo client_type en el cuerpo del login antes de procesarlo.
//
// Parámetros:
//   - ct: cadena a validar (por ejemplo "web", "mobile", "desktop").
//
// Retorna true si ct corresponde a un ClientType válido, false en caso contrario.
func IsValidClientType(ct string) bool {
	switch ClientType(ct) {
	case ClientTypeWeb, ClientTypeMobile, ClientTypeDesktop:
		return true
	}
	return false
}

// Claims representa el payload del JWT de acceso emitido por Sentinel.
// Estos campos se incluyen en el token firmado con RS256 que los backends
// consumidores validan localmente usando la clave pública (/.well-known/jwks.json).
// El token de acceso tiene una duración corta (por defecto 60 minutos).
type Claims struct {
	// Sub es el identificador del sujeto del token (Subject), corresponde al UUID del usuario.
	// Es el campo estándar JWT para identificar de forma única a quien se emitió el token.
	Sub string `json:"sub"`

	// Username es el nombre de usuario legible, incluido para que los backends consumidores
	// puedan mostrar información del usuario sin consultar Sentinel.
	Username string `json:"username"`

	// Email es la dirección de correo del usuario, incluida para los mismos fines que Username.
	Email string `json:"email"`

	// App es el slug de la aplicación para la que se emitió el token.
	// Los backends consumidores deben verificar que este valor coincida con su propio slug.
	App string `json:"app"`

	// Roles es la lista de nombres de roles activos del usuario en la aplicación.
	// Se incluye en el token para evitar consultas adicionales en operaciones frecuentes.
	Roles []string `json:"roles"`

	// Iat es la marca de tiempo de emisión del token (Issued At), en segundos Unix.
	// Corresponde al claim estándar JWT "iat".
	Iat int64 `json:"iat"`

	// Exp es la marca de tiempo de expiración del token (Expiration), en segundos Unix.
	// Corresponde al claim estándar JWT "exp". Los backends deben rechazar tokens expirados.
	Exp int64 `json:"exp"`

	// Jti es el identificador único del token (JWT ID), generado como UUID v4.
	// Permite revocar tokens individuales o detectar reutilización si se implementa
	// una lista de tokens revocados. Corresponde al claim estándar JWT "jti".
	Jti string `json:"jti"`
}

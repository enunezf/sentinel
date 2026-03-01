// Package domain contiene los tipos de datos centrales del sistema Sentinel.
// Estos structs representan las entidades del negocio tal como se almacenan
// en la base de datos y se intercambian entre las capas de servicio y repositorio.
// No deben contener lógica de infraestructura (SQL, HTTP, etc.).
package domain

import (
	"time"

	"github.com/google/uuid"
)

// User representa una cuenta de usuario registrada en Sentinel.
// Cada usuario puede autenticarse con usuario+contraseña, recibir roles
// y permisos directos, y estar asociado a centros de costo.
// La contraseña nunca se almacena en texto plano; solo se guarda el hash bcrypt
// en el campo PasswordHash.
type User struct {
	// ID es el identificador único del usuario (UUID v4).
	// Se genera automáticamente al crear el registro en la base de datos.
	ID uuid.UUID

	// Username es el nombre de usuario utilizado para el inicio de sesión.
	// Es único en el sistema y no puede cambiarse después de la creación.
	Username string

	// Email es la dirección de correo electrónico del usuario.
	// Se utiliza para notificaciones y como identificador alternativo.
	Email string

	// PasswordHash almacena el hash bcrypt (costo >= 12) de la contraseña del usuario.
	// Nunca se almacena la contraseña en texto plano.
	// Antes de hashear, la contraseña se normaliza a Unicode NFC.
	PasswordHash string

	// IsActive indica si la cuenta está habilitada para iniciar sesión.
	// Una cuenta inactiva no puede autenticarse aunque las credenciales sean correctas.
	IsActive bool

	// MustChangePwd indica que el usuario debe cambiar su contraseña en el próximo inicio de sesión.
	// Se activa cuando un administrador crea o restablece la contraseña de un usuario.
	MustChangePwd bool

	// LastLoginAt registra la fecha y hora del último inicio de sesión exitoso.
	// Es nil si el usuario nunca ha iniciado sesión. Se usa para auditoría.
	LastLoginAt *time.Time

	// FailedAttempts es el contador de intentos de autenticación fallidos consecutivos.
	// Se reinicia a cero cuando el usuario inicia sesión exitosamente.
	// Al superar el máximo configurado (por defecto 5), la cuenta se bloquea temporalmente.
	FailedAttempts int

	// LockedUntil es la fecha y hora hasta la cual la cuenta está bloqueada.
	// Un valor nil combinado con LockoutCount >= 3 indica bloqueo permanente.
	// Un valor futuro indica bloqueo temporal.
	LockedUntil *time.Time

	// LockoutCount es el número de veces que la cuenta ha sido bloqueada en el día actual
	// (medido por LockoutDate). Cuando llega a 3 en el mismo día, el bloqueo se vuelve permanente.
	LockoutCount int

	// LockoutDate es la fecha (sin hora) del último bloqueo registrado.
	// Se usa para determinar si los LockoutCount pertenecen al mismo día.
	// En PostgreSQL se almacena como tipo DATE; aquí se mapea a time.Time truncado al día.
	LockoutDate *time.Time // stored as DATE in PG, mapped to time.Time truncated to day

	// CreatedAt es la fecha y hora de creación del registro en la base de datos.
	CreatedAt time.Time

	// UpdatedAt es la fecha y hora de la última modificación del registro.
	// Se actualiza automáticamente en cada operación de escritura.
	UpdatedAt time.Time
}

// IsLocked informa si la cuenta del usuario está bloqueada en el instante now.
//
// Existen dos modalidades de bloqueo:
//   - Temporal: LockedUntil tiene un valor futuro. El bloqueo expira automáticamente.
//   - Permanente: LockedUntil es nil Y el usuario sufrió 3 o más bloqueos en el mismo día
//     (LockoutCount >= 3 y LockoutDate no es nil). Requiere desbloqueo manual por un administrador.
//
// Parámetros:
//   - now: instante de tiempo contra el cual se evalúa el bloqueo temporal.
//
// Retorna true si la cuenta está bloqueada (temporal o permanentemente).
func (u *User) IsLocked(now time.Time) bool {
	if u.LockedUntil == nil {
		// Bloqueo permanente: lockout_count >= 3 y locked_until IS NULL (fue seteado así).
		// Detectamos bloqueo permanente verificando que LockedUntil sea nil Y lockoutCount >= 3
		// Y que el usuario haya sido efectivamente bloqueado alguna vez (no solo creado sin lockouts).
		// Usamos LockoutDate como señal de que ocurrió al menos un bloqueo.
		return u.LockoutCount >= 3 && u.LockoutDate != nil
	}
	return now.Before(*u.LockedUntil)
}

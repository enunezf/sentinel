// Package domain contiene los tipos de datos centrales del sistema Sentinel.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Application representa una aplicación cliente registrada en Sentinel (tenant).
// Cada aplicación que desee usar el servicio de autenticación debe estar registrada
// y poseer una clave secreta (SecretKey) que se envía en el header X-App-Key.
// Este mecanismo permite que múltiples sistemas internos compartan la misma
// infraestructura de autenticación de forma aislada.
type Application struct {
	// ID es el identificador único de la aplicación (UUID v4).
	// Se genera automáticamente al crear el registro.
	ID uuid.UUID

	// Name es el nombre descriptivo de la aplicación, legible por humanos.
	// Por ejemplo: "Sistema ERP" o "Portal de Clientes".
	Name string

	// Slug es un identificador corto, en minúsculas, sin espacios, único en el sistema.
	// Se incluye en el claim "app" del JWT para que los backends consumidores
	// identifiquen la aplicación sin necesidad de consultar la base de datos.
	Slug string

	// SecretKey es la clave secreta que la aplicación cliente debe enviar
	// en el header HTTP "X-App-Key" para autenticar cada solicitud al servicio.
	// Se genera como UUID v4 al crear la aplicación y puede rotarse con el
	// endpoint /admin/applications/:id/rotate-key.
	SecretKey string

	// IsActive indica si la aplicación está habilitada para recibir solicitudes.
	// Una aplicación inactiva tendrá todas sus peticiones rechazadas con 401.
	IsActive bool

	// CreatedAt es la fecha y hora de creación del registro.
	CreatedAt time.Time

	// UpdatedAt es la fecha y hora de la última modificación del registro.
	UpdatedAt time.Time
}

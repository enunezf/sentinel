// Package config gestiona la carga, expansión de variables de entorno y validación
// de la configuración del servicio Sentinel.
//
// El archivo de configuración principal es config.yaml (ubicado en el directorio de trabajo
// o en la ruta indicada por la variable de entorno CONFIG_PATH). Los valores sensibles
// como contraseñas y rutas de claves se inyectan mediante variables de entorno usando
// la sintaxis ${NOMBRE_VAR} dentro del YAML.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config agrupa toda la configuración de la aplicación Sentinel.
// Cada sección corresponde a un bloque de primer nivel en el archivo config.yaml.
type Config struct {
	// Server contiene los parámetros del servidor HTTP Fiber.
	Server ServerConfig `yaml:"server"`

	// Database contiene los parámetros de conexión a PostgreSQL.
	Database DatabaseConfig `yaml:"database"`

	// Redis contiene los parámetros de conexión a Redis.
	Redis RedisConfig `yaml:"redis"`

	// JWT contiene las rutas de las claves RSA y los TTL de los tokens.
	JWT JWTConfig `yaml:"jwt"`

	// Security contiene las políticas de seguridad: intentos de login, lockout, bcrypt.
	Security SecurityConfig `yaml:"security"`

	// Bootstrap contiene las credenciales del usuario administrador inicial.
	Bootstrap BootstrapConfig `yaml:"bootstrap"`

	// Logging contiene los parámetros del logger estructurado.
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig contiene los parámetros de operación del servidor HTTP.
type ServerConfig struct {
	// Port es el puerto TCP en el que el servidor escuchará conexiones entrantes.
	// Valor por defecto: 8080.
	Port int `yaml:"port"`

	// ReadTimeout es el tiempo máximo para leer el cuerpo de una solicitud HTTP entrante.
	// Valor por defecto: 30s. Protege contra clientes lentos que mantengan conexiones abiertas.
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout es el tiempo máximo para escribir la respuesta HTTP al cliente.
	// Valor por defecto: 30s.
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// GracefulShutdownTimeout es el tiempo máximo que se espera para que las solicitudes
	// en curso terminen cuando se recibe una señal de apagado (SIGINT/SIGTERM).
	// Valor por defecto: 15s. Pasado este tiempo, el servidor fuerza el cierre.
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

// DatabaseConfig contiene los parámetros de conexión al pool de PostgreSQL (pgx).
type DatabaseConfig struct {
	// Host es la dirección del servidor PostgreSQL. Puede ser un nombre de host o IP.
	Host string `yaml:"host"`

	// Port es el puerto TCP de PostgreSQL. Valor por defecto: 5432.
	Port int `yaml:"port"`

	// Name es el nombre de la base de datos a usar.
	Name string `yaml:"name"`

	// User es el nombre del usuario de base de datos para la conexión.
	User string `yaml:"user"`

	// Password es la contraseña del usuario de base de datos.
	// Se recomienda inyectarla mediante variable de entorno: ${DB_PASSWORD}.
	Password string `yaml:"password"`

	// MaxOpenConns es el número máximo de conexiones simultáneas en el pool.
	// Valor por defecto: 50. Ajustar según la capacidad del servidor PostgreSQL.
	MaxOpenConns int `yaml:"max_open_conns"`

	// MaxIdleConns es el número mínimo de conexiones mantenidas abiertas (idle) en el pool.
	// Valor por defecto: 10. Reduce la latencia de la primera conexión en periodos de baja carga.
	MaxIdleConns int `yaml:"max_idle_conns"`

	// ConnMaxLifetime es el tiempo máximo de vida de una conexión en el pool antes de ser cerrada
	// y reemplazada. Valor por defecto: 5 minutos. Evita conexiones obsoletas o con fallos silenciosos.
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// DSN construye y retorna una cadena de conexión compatible con pgx/pgxpool.
// El modo SSL se deshabilita explícitamente (sslmode=disable) porque Sentinel
// asume que la conexión a la base de datos ocurre dentro de una red privada segura.
// En entornos productivos con PostgreSQL externo, se recomienda habilitar TLS.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		d.Host, d.Port, d.Name, d.User, d.Password,
	)
}

// RedisConfig contiene los parámetros de conexión al servidor Redis.
// Redis se usa en Sentinel para almacenar refresh tokens (con TTL automático)
// y para la caché de permisos por usuario.
type RedisConfig struct {
	// Addr es la dirección del servidor Redis en formato "host:puerto".
	// Ejemplo: "localhost:6379".
	Addr string `yaml:"addr"`

	// Password es la contraseña de Redis. Puede ser vacía si Redis no requiere autenticación.
	// Se recomienda inyectarla mediante variable de entorno: ${REDIS_PASSWORD}.
	Password string `yaml:"password"`

	// DB es el número de base de datos lógica de Redis a usar (0-15).
	// Valor 0 es la base de datos predeterminada.
	DB int `yaml:"db"`
}

// JWTConfig contiene la configuración para la generación y validación de tokens JWT.
// Sentinel usa exclusivamente el algoritmo RS256 (RSA + SHA-256) para firmar tokens,
// lo que permite a los backends consumidores validarlos localmente con la clave pública
// sin necesidad de contactar a Sentinel en cada petición.
type JWTConfig struct {
	// PrivateKeyPath es la ruta al archivo PEM que contiene la clave privada RSA.
	// Se usa para firmar los access tokens. Nunca debe incluirse en la imagen Docker;
	// se monta como volumen en tiempo de ejecución.
	PrivateKeyPath string `yaml:"private_key_path"`

	// PublicKeyPath es la ruta al archivo PEM que contiene la clave pública RSA.
	// Se usa para validar tokens y se expone en el endpoint /.well-known/jwks.json.
	PublicKeyPath string `yaml:"public_key_path"`

	// AccessTokenTTL es el tiempo de vida del access token JWT.
	// Valor por defecto: 60 minutos. Un TTL corto limita el impacto de tokens robados.
	AccessTokenTTL time.Duration `yaml:"access_token_ttl"`

	// RefreshTokenTTLWeb es el tiempo de vida del refresh token para clientes web.
	// Valor por defecto: 168 horas (7 días). Los navegadores cierran sesión semanalmente.
	RefreshTokenTTLWeb time.Duration `yaml:"refresh_token_ttl_web"`

	// RefreshTokenTTLMobile es el tiempo de vida del refresh token para clientes móvil y desktop.
	// Valor por defecto: 720 horas (30 días). Las apps móviles y de escritorio requieren
	// sesiones más largas para mejorar la experiencia de usuario.
	RefreshTokenTTLMobile time.Duration `yaml:"refresh_token_ttl_mobile"`
}

// SecurityConfig contiene las políticas de seguridad para autenticación y contraseñas.
type SecurityConfig struct {
	// MaxFailedAttempts es el número máximo de intentos de login fallidos consecutivos
	// permitidos antes de bloquear la cuenta temporalmente.
	// Valor por defecto: 5. Al alcanzarlo, la cuenta se bloquea por LockoutDuration.
	MaxFailedAttempts int `yaml:"max_failed_attempts"`

	// LockoutDuration es la duración del bloqueo temporal después de superar MaxFailedAttempts.
	// Valor por defecto: 15 minutos. Si ocurren 3 bloqueos en el mismo día,
	// el siguiente bloqueo es permanente y requiere intervención del administrador.
	LockoutDuration time.Duration `yaml:"lockout_duration"`

	// BcryptCost es el factor de costo para el hash bcrypt de contraseñas.
	// Valor por defecto: 12. Valores más altos son más seguros pero más lentos;
	// el costo 12 ofrece un buen balance para hardware moderno (~300ms por hash).
	BcryptCost int `yaml:"bcrypt_cost"`

	// PasswordHistory es el número de contraseñas anteriores que se verifican
	// al cambiar la contraseña. Impide reusar contraseñas recientes.
	// Valor por defecto: 5 (no se puede reusar ninguna de las últimas 5 contraseñas).
	PasswordHistory int `yaml:"password_history"`
}

// BootstrapConfig contiene las credenciales del usuario administrador que se crea
// automáticamente la primera vez que el sistema arranca con la tabla applications vacía.
// La contraseña de bootstrap está exenta de las políticas de complejidad normales.
type BootstrapConfig struct {
	// AdminUser es el nombre de usuario del administrador inicial del sistema.
	// Se inyecta mediante la variable de entorno BOOTSTRAP_ADMIN_USER.
	AdminUser string `yaml:"admin_user"`

	// AdminPassword es la contraseña en texto plano del administrador inicial.
	// Se hasheará con bcrypt durante el bootstrap y nunca se almacenará en texto plano.
	// Se inyecta mediante la variable de entorno BOOTSTRAP_ADMIN_PASSWORD.
	AdminPassword string `yaml:"admin_password"`
}

// LoggingConfig contiene los parámetros del logger estructurado (slog).
type LoggingConfig struct {
	// Level es el nivel mínimo de severidad que se registrará.
	// Valores permitidos: "debug", "info", "warn", "error". Valor por defecto: "info".
	Level string `yaml:"level"` // debug, info, warn, error

	// Format es el formato de salida de los logs.
	// "json" produce logs estructurados (ideal para agregadores como Loki o CloudWatch).
	// "text" produce logs legibles por humanos (ideal para desarrollo local).
	// Valor por defecto: "json".
	Format string `yaml:"format"` // json, text

	// Output determina el destino de los logs.
	// "stderr" escribe en la salida de error estándar; cualquier otro valor usa stdout.
	// Valor por defecto: "stdout".
	Output string `yaml:"output"` // stdout, stderr (default: stdout)
}

// envVarRegexp es la expresión regular que detecta placeholders ${VAR_NAME} en el YAML.
// Se compila una sola vez al iniciar el paquete para eficiencia.
var envVarRegexp = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars reemplaza cada placeholder ${VAR_NAME} en s con el valor de la variable
// de entorno correspondiente. Si la variable no está definida, se sustituye por cadena vacía.
// Esta función permite mantener secretos fuera del archivo de configuración YAML.
//
// Parámetros:
//   - s: cadena YAML que puede contener uno o más placeholders ${...}.
//
// Retorna la cadena con todos los placeholders reemplazados por sus valores de entorno.
func expandEnvVars(s string) string {
	return envVarRegexp.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		val := os.Getenv(name)
		return val
	})
}

// rawConfig es un tipo auxiliar para deserializar el YAML antes de expandir variables.
// No se usa directamente en la lógica principal; está definido para posibles usos futuros
// de pre-procesamiento antes de unmarshal al tipo Config.
type rawConfig map[string]interface{}

// Load lee el archivo de configuración en path, expande los placeholders de variables
// de entorno, aplica valores por defecto para campos no especificados y valida que
// todos los campos obligatorios estén presentes.
//
// Parámetros:
//   - path: ruta al archivo config.yaml. Si la variable de entorno CONFIG_PATH está definida,
//     se usa esa ruta en lugar del valor por defecto "config.yaml".
//
// Retorna un puntero a Config listo para usar, o un error descriptivo si algo falla.
// Los posibles errores incluyen: archivo no encontrado, YAML inválido, o campos requeridos vacíos.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: cannot read file %q: %w", path, err)
	}

	// Sustituir placeholders ${VAR} por valores de variables de entorno antes de parsear.
	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: cannot parse YAML: %w", err)
	}

	// Aplicar valores por defecto para campos no especificados en el YAML.
	// Se verifica el valor cero de cada tipo (0 para int/Duration, "" para string).
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}
	if cfg.Server.GracefulShutdownTimeout == 0 {
		cfg.Server.GracefulShutdownTimeout = 15 * time.Second
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.Database.MaxOpenConns == 0 {
		cfg.Database.MaxOpenConns = 50
	}
	if cfg.Database.MaxIdleConns == 0 {
		cfg.Database.MaxIdleConns = 10
	}
	if cfg.Database.ConnMaxLifetime == 0 {
		cfg.Database.ConnMaxLifetime = 5 * time.Minute
	}
	if cfg.JWT.AccessTokenTTL == 0 {
		cfg.JWT.AccessTokenTTL = 60 * time.Minute
	}
	if cfg.JWT.RefreshTokenTTLWeb == 0 {
		// 7 días en horas
		cfg.JWT.RefreshTokenTTLWeb = 168 * time.Hour
	}
	if cfg.JWT.RefreshTokenTTLMobile == 0 {
		// 30 días en horas
		cfg.JWT.RefreshTokenTTLMobile = 720 * time.Hour
	}
	if cfg.Security.BcryptCost == 0 {
		cfg.Security.BcryptCost = 12
	}
	if cfg.Security.MaxFailedAttempts == 0 {
		cfg.Security.MaxFailedAttempts = 5
	}
	if cfg.Security.LockoutDuration == 0 {
		cfg.Security.LockoutDuration = 15 * time.Minute
	}
	if cfg.Security.PasswordHistory == 0 {
		cfg.Security.PasswordHistory = 5
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "stdout"
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate verifica que todos los campos obligatorios de la configuración tengan
// valores no vacíos. Acumula todos los errores encontrados en un único mensaje
// para facilitar la corrección de múltiples problemas en un solo intento.
//
// Parámetros:
//   - cfg: puntero a la configuración ya deserializada y con valores por defecto aplicados.
//
// Retorna nil si la configuración es válida, o un error con la lista completa de
// campos faltantes si hay algún problema.
func validate(cfg *Config) error {
	var errs []string

	if cfg.Database.Host == "" {
		errs = append(errs, "database.host (DB_HOST) is required")
	}
	if cfg.Database.Name == "" {
		errs = append(errs, "database.name (DB_NAME) is required")
	}
	if cfg.Database.User == "" {
		errs = append(errs, "database.user (DB_USER) is required")
	}
	if cfg.Database.Password == "" {
		errs = append(errs, "database.password (DB_PASSWORD) is required")
	}
	if cfg.Redis.Addr == "" {
		errs = append(errs, "redis.addr (REDIS_ADDR) is required")
	}
	if cfg.JWT.PrivateKeyPath == "" {
		errs = append(errs, "jwt.private_key_path (JWT_PRIVATE_KEY_PATH) is required")
	}
	if cfg.JWT.PublicKeyPath == "" {
		errs = append(errs, "jwt.public_key_path (JWT_PUBLIC_KEY_PATH) is required")
	}
	if cfg.Bootstrap.AdminUser == "" {
		errs = append(errs, "bootstrap.admin_user (BOOTSTRAP_ADMIN_USER) is required")
	}
	if cfg.Bootstrap.AdminPassword == "" {
		errs = append(errs, "bootstrap.admin_password (BOOTSTRAP_ADMIN_PASSWORD) is required")
	}

	if len(errs) > 0 {
		return errors.New("config validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}

	return nil
}

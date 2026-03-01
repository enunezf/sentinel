package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/token"
)

// AuthzService implementa la lógica de negocio de autorización de Sentinel.
// Gestiona tres operaciones principales:
//  1. Verify: comprueba si el usuario tiene un permiso específico (con o sin restricción
//     de centro de costo).
//  2. GetUserPermissions: devuelve el contexto completo de permisos del usuario
//     (roles, permisos, centros de costo, roles temporales).
//  3. GetPermissionsMap: genera y firma el mapa canónico de permisos de una aplicación,
//     usado por backends consumidores para verificación local.
//
// El contexto de usuario se almacena en Redis con el JTI del access token como clave;
// el TTL coincide con el del token para que el caché expire automáticamente.
type AuthzService struct {
	appRepo      *postgres.ApplicationRepository      // resolución de app por slug
	userRoleRepo *postgres.UserRoleRepository         // roles activos del usuario
	userPermRepo *postgres.UserPermissionRepository   // permisos individuales del usuario
	userCCRepo   *postgres.UserCostCenterRepository   // centros de costo asignados al usuario
	permRepo     *postgres.PermissionRepository       // catálogo de permisos de la aplicación
	roleRepo     *postgres.RoleRepository             // catálogo de roles y sus permisos
	ccRepo       *postgres.CostCenterRepository       // catálogo de centros de costo
	authzCache   *redisrepo.AuthzCache                // caché de contextos de usuario y mapas de permisos
	tokenMgr     *token.Manager                      // firma RSA-SHA256 del mapa de permisos
	auditSvc     *AuditService                       // registro asíncrono de decisiones de autorización
}

// NewAuthzService crea un AuthzService con todas las dependencias necesarias.
func NewAuthzService(
	appRepo *postgres.ApplicationRepository,
	userRoleRepo *postgres.UserRoleRepository,
	userPermRepo *postgres.UserPermissionRepository,
	userCCRepo *postgres.UserCostCenterRepository,
	permRepo *postgres.PermissionRepository,
	roleRepo *postgres.RoleRepository,
	ccRepo *postgres.CostCenterRepository,
	authzCache *redisrepo.AuthzCache,
	tokenMgr *token.Manager,
	auditSvc *AuditService,
) *AuthzService {
	return &AuthzService{
		appRepo:      appRepo,
		userRoleRepo: userRoleRepo,
		userPermRepo: userPermRepo,
		userCCRepo:   userCCRepo,
		permRepo:     permRepo,
		roleRepo:     roleRepo,
		ccRepo:       ccRepo,
		authzCache:   authzCache,
		tokenMgr:     tokenMgr,
		auditSvc:     auditSvc,
	}
}

// VerifyRequest contiene los parámetros de una consulta de autorización.
type VerifyRequest struct {
	Permission   string // código del permiso a verificar (ej: "admin.users.read")
	CostCenterID string // código del centro de costo requerido; vacío = sin restricción
}

// VerifyResponse contiene el resultado de una consulta de autorización.
type VerifyResponse struct {
	Allowed     bool      `json:"allowed"`      // true si el usuario tiene el permiso (y el centro de costo, si se especificó)
	UserID      string    `json:"user_id"`      // UUID del usuario evaluado
	Username    string    `json:"username"`     // nombre del usuario evaluado
	Permission  string    `json:"permission"`   // código del permiso consultado
	EvaluatedAt time.Time `json:"evaluated_at"` // instante UTC en que se evaluó la decisión
}

// Verify evalúa si el usuario autenticado tiene el permiso indicado, con la restricción
// opcional de centro de costo.
// El contexto de usuario se obtiene de Redis (caché) o se construye desde PostgreSQL
// si no está en caché.
// Siempre emite un evento de auditoría: EventAuthzPermissionGranted o EventAuthzPermissionDenied.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - claims: claims del JWT del usuario autenticado.
//   - req: permiso y centro de costo a verificar.
//   - ip, ua: datos del cliente para auditoría.
func (s *AuthzService) Verify(ctx context.Context, claims *domain.Claims, req VerifyRequest, ip, ua string) (*VerifyResponse, error) {
	userID, _ := uuid.Parse(claims.Sub)
	appID, err := s.resolveAppID(ctx, claims.App)
	if err != nil {
		return nil, err
	}

	// Obtener o construir el contexto de usuario (caché Redis primero).
	uc, err := s.GetOrBuildUserContext(ctx, claims, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get user context: %w", err)
	}

	allowed := s.hasPermission(uc, req.Permission)

	// Si el permiso existe, verificar adicionalmente la restricción de centro de costo.
	if allowed && req.CostCenterID != "" {
		allowed = s.hasCostCenter(uc, req.CostCenterID)
	}

	// Registrar la decisión de autorización en auditoría.
	et := domain.EventAuthzPermissionGranted
	if !allowed {
		et = domain.EventAuthzPermissionDenied
	}
	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     et,
		ApplicationID: &appID,
		UserID:        &userID,
		ActorID:       &userID,
		ResourceType:  &resType,
		NewValue:      map[string]interface{}{"permission": req.Permission, "allowed": allowed},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       allowed,
	})

	return &VerifyResponse{
		Allowed:     allowed,
		UserID:      claims.Sub,
		Username:    claims.Username,
		Permission:  req.Permission,
		EvaluatedAt: time.Now().UTC(),
	}, nil
}

// hasPermission busca linealmente el código de permiso en el slice del contexto de usuario.
// La búsqueda lineal es aceptable porque el número de permisos por usuario es pequeño
// (generalmente < 50).
func (s *AuthzService) hasPermission(uc *redisrepo.UserContext, permCode string) bool {
	for _, p := range uc.Permissions {
		if p == permCode {
			return true
		}
	}
	return false
}

// hasCostCenter verifica si el código de centro de costo está en la lista asignada
// al usuario. Se llama solo cuando hasPermission ya devolvió true.
func (s *AuthzService) hasCostCenter(uc *redisrepo.UserContext, ccCode string) bool {
	for _, cc := range uc.CostCenters {
		if cc == ccCode {
			return true
		}
	}
	return false
}

// GetOrBuildUserContext devuelve el contexto de usuario desde Redis (caché) si existe,
// o lo construye desde PostgreSQL y lo almacena en caché con un TTL de 60 minutos
// (valor fijo que coincide aproximadamente con el TTL del access token).
//
// La clave de caché es el JTI del access token, por lo que el contexto expira
// automáticamente cuando el token ya no es válido.
func (s *AuthzService) GetOrBuildUserContext(ctx context.Context, claims *domain.Claims, appID uuid.UUID) (*redisrepo.UserContext, error) {
	// Intentar obtener del caché primero.
	cached, err := s.authzCache.GetPermissions(ctx, claims.Jti)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Cache miss: construir desde PostgreSQL.
	uc, err := s.buildUserContext(ctx, claims, appID)
	if err != nil {
		return nil, err
	}

	// Almacenar en caché. El TTL de 60 minutos es un valor fijo; para mayor precisión
	// se podría calcular el tiempo restante del access token desde claims.Exp.
	ttl := 60 * time.Minute
	_ = s.authzCache.SetPermissions(ctx, claims.Jti, uc, ttl)
	return uc, nil
}

// buildUserContext construye el contexto de autorización del usuario desde PostgreSQL.
// Combina dos fuentes de permisos:
//  1. Permisos de roles: obtenidos a través de los roles activos del usuario en la app.
//  2. Permisos individuales: asignados directamente al usuario (sin pasar por un rol).
//
// La unión se realiza con un map[string]struct{} para eliminar duplicados.
// El resultado se ordena lexicográficamente para producir salidas deterministas.
func (s *AuthzService) buildUserContext(ctx context.Context, claims *domain.Claims, appID uuid.UUID) (*redisrepo.UserContext, error) {
	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("authz: invalid user id: %w", err)
	}

	// Usar un set para eliminar duplicados entre permisos de roles y permisos directos.
	permSet := make(map[string]struct{})

	// Fuente 1: permisos heredados de los roles activos del usuario en esta aplicación.
	rolePerms, err := s.getRolePermissionsForUser(ctx, userID, appID)
	if err != nil {
		return nil, err
	}
	for _, p := range rolePerms {
		permSet[p] = struct{}{}
	}

	// Fuente 2: permisos individuales asignados directamente al usuario.
	indivPerms, err := s.userPermRepo.GetActivePermissionCodesForUserApp(ctx, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get individual permissions: %w", err)
	}
	for _, p := range indivPerms {
		permSet[p] = struct{}{}
	}

	// Convertir el set a slice ordenado para salida determinista.
	permissions := make([]string, 0, len(permSet))
	for p := range permSet {
		permissions = append(permissions, p)
	}
	sort.Strings(permissions)

	// Obtener los centros de costo activos asignados al usuario en esta aplicación.
	costCenters, err := s.userCCRepo.GetActiveCodesForUserApp(ctx, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get cost centers: %w", err)
	}

	return &redisrepo.UserContext{
		UserID:      claims.Sub,
		Application: claims.App,
		Roles:       claims.Roles,       // roles ya incluidos en el JWT
		Permissions: permissions,
		CostCenters: costCenters,
	}, nil
}

// getRolePermissionsForUser devuelve todos los códigos de permisos provenientes de
// los roles activos del usuario en la aplicación indicada.
// Estrategia:
//  1. Obtener los nombres de los roles activos del usuario.
//  2. Para cada rol, buscar su entidad completa y obtener sus permisos.
//  3. Combinar todos los permisos en un set para eliminar duplicados.
//
// Nota: esta implementación hace N+1 queries por rol; es aceptable para la escala actual.
// En el futuro se puede optimizar con una sola query JOIN.
func (s *AuthzService) getRolePermissionsForUser(ctx context.Context, userID, appID uuid.UUID) ([]string, error) {
	const q = `
		SELECT DISTINCT p.code
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN role_permissions rp ON rp.role_id = r.id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
		  AND ur.application_id = $2
		  AND ur.is_active = TRUE
		  AND r.is_active = TRUE
		  AND ur.valid_from <= NOW()
		  AND (ur.valid_until IS NULL OR ur.valid_until > NOW())`
	// La query anterior es conceptual; la implementación actual usa los repositorios
	// existentes para evitar acceso directo al DB desde el servicio.
	_ = q

	// Obtener los nombres de los roles activos del usuario para esta aplicación.
	roleNames, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get role names: %w", err)
	}
	if len(roleNames) == 0 {
		return nil, nil
	}

	// Para cada rol activo, obtener sus permisos y acumular en un set.
	permSet := make(map[string]struct{})
	for _, roleName := range roleNames {
		role, err := s.roleRepo.FindByNameAndApp(ctx, roleName, appID)
		if err != nil || role == nil {
			continue
		}
		perms, err := s.roleRepo.GetPermissions(ctx, role.ID)
		if err != nil {
			continue
		}
		for _, p := range perms {
			permSet[p.Code] = struct{}{}
		}
	}

	codes := make([]string, 0, len(permSet))
	for c := range permSet {
		codes = append(codes, c)
	}
	return codes, nil
}

// MePermissionsResponse contiene el contexto completo de autorización del usuario.
type MePermissionsResponse struct {
	UserID         string          `json:"user_id"`         // UUID del usuario
	Application    string          `json:"application"`     // slug de la aplicación
	Roles          []string        `json:"roles"`           // nombres de los roles activos
	Permissions    []string        `json:"permissions"`     // códigos de permisos (roles + individuales, deduplicados)
	CostCenters    []string        `json:"cost_centers"`    // códigos de centros de costo activos
	TemporaryRoles []TemporaryRole `json:"temporary_roles"` // roles con valid_until definido y aún vigentes
}

// TemporaryRole describe un rol con vigencia limitada asignado al usuario.
type TemporaryRole struct {
	Role       string    `json:"role"`        // nombre del rol
	ValidUntil time.Time `json:"valid_until"` // fecha de expiración del rol
}

// GetUserPermissions construye y devuelve el contexto completo de permisos del usuario,
// incluyendo roles temporales (aquellos con valid_until definido y aún vigentes).
// Usa el caché de Redis cuando está disponible.
func (s *AuthzService) GetUserPermissions(ctx context.Context, claims *domain.Claims) (*MePermissionsResponse, error) {
	appID, err := s.resolveAppID(ctx, claims.App)
	if err != nil {
		return nil, err
	}

	uc, err := s.GetOrBuildUserContext(ctx, claims, appID)
	if err != nil {
		return nil, err
	}

	// Buscar roles temporales: activos, con valid_until definido y en el rango válido.
	userID, _ := uuid.Parse(claims.Sub)
	userRoles, err := s.userRoleRepo.ListForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("authz: list user roles: %w", err)
	}

	var tempRoles []TemporaryRole
	for _, ur := range userRoles {
		if ur.ValidUntil != nil && ur.IsActive &&
			ur.ApplicationID == appID &&
			ur.ValidFrom.Before(time.Now()) &&
			ur.ValidUntil.After(time.Now()) {
			tempRoles = append(tempRoles, TemporaryRole{
				Role:       ur.RoleName,
				ValidUntil: *ur.ValidUntil,
			})
		}
	}

	return &MePermissionsResponse{
		UserID:         uc.UserID,
		Application:    uc.Application,
		Roles:          uc.Roles,
		Permissions:    uc.Permissions,
		CostCenters:    uc.CostCenters,
		TemporaryRoles: tempRoles,
	}, nil
}

// PermissionMapEntry describe un permiso en el mapa canónico de una aplicación.
type PermissionMapEntry struct {
	Roles       []string `json:"roles"`       // nombres de los roles que tienen este permiso
	Description string   `json:"description"` // descripción legible del permiso
}

// CostCenterMapEntry describe un centro de costo en el mapa canónico de una aplicación.
type CostCenterMapEntry struct {
	Code     string `json:"code"`      // código único del centro de costo
	Name     string `json:"name"`      // nombre legible
	IsActive bool   `json:"is_active"` // indica si está activo
}

// PermissionsMapResponse es el mapa firmado de permisos para una aplicación.
// Los backends consumidores pueden verificar la firma con la clave pública del JWKS
// para validar la integridad del mapa sin necesidad de consultar a Sentinel.
type PermissionsMapResponse struct {
	Application string                        `json:"application"` // slug de la aplicación
	GeneratedAt time.Time                     `json:"generated_at"` // instante UTC de generación
	Version     string                        `json:"version"`     // hash SHA-256 truncado (8 hex) del payload canónico
	Permissions map[string]PermissionMapEntry `json:"permissions"` // mapa código -> descripción + roles
	CostCenters map[string]CostCenterMapEntry `json:"cost_centers"` // mapa código -> datos del centro
	Signature   string                        `json:"signature"`   // firma RSA-SHA256 en base64url del payload canónico
}

// GetPermissionsMap construye y firma el mapa de permisos para una aplicación.
// El resultado se almacena en Redis con TTL de 5 minutos. Si hay un hit de caché,
// se devuelve directamente sin consultar PostgreSQL.
//
// El mapa incluye todos los permisos de la aplicación con los roles que los tienen
// y todos los centros de costo. La firma RSA-SHA256 permite a los backends
// consumidores verificar la integridad sin depender de Sentinel en cada request.
func (s *AuthzService) GetPermissionsMap(ctx context.Context, appSlug string) (*PermissionsMapResponse, error) {
	app, err := s.appRepo.FindBySlug(ctx, appSlug)
	if err != nil || app == nil {
		return nil, fmt.Errorf("authz: app not found: %s", appSlug)
	}

	// Intentar obtener del caché Redis.
	cached, err := s.authzCache.GetPermissionsMap(ctx, appSlug)
	if err == nil && len(cached) > 0 {
		var resp PermissionsMapResponse
		if err := json.Unmarshal(cached, &resp); err == nil {
			return &resp, nil
		}
	}

	resp, err := s.buildPermissionsMap(ctx, app)
	if err != nil {
		return nil, err
	}

	// Almacenar en caché el mapa serializado y la versión por separado.
	b, _ := json.Marshal(resp)
	_ = s.authzCache.SetPermissionsMap(ctx, appSlug, b, 5*time.Minute)
	_ = s.authzCache.SetPermissionsMapVersion(ctx, appSlug, resp.Version, 5*time.Minute)

	return resp, nil
}

// buildPermissionsMap ensambla el mapa de permisos completo desde PostgreSQL.
// Proceso:
//  1. Listar todos los permisos de la aplicación.
//  2. Para cada permiso, obtener los roles que lo tienen.
//  3. Listar todos los centros de costo de la aplicación.
//  4. Serializar en formato canónico (claves ordenadas, sin espacios).
//  5. Calcular la versión como SHA-256 del payload canónico (primeros 4 bytes = 8 hex).
//  6. Firmar el payload con RSA-SHA256 usando la clave privada de Sentinel.
func (s *AuthzService) buildPermissionsMap(ctx context.Context, app *domain.Application) (*PermissionsMapResponse, error) {
	perms, err := s.permRepo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("authz: list permissions: %w", err)
	}

	ccs, err := s.ccRepo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("authz: list cost centers: %w", err)
	}

	// Construir el mapa permiso -> roles.
	permMap := make(map[string]PermissionMapEntry)
	for _, p := range perms {
		roles, err := s.getRolesForPermission(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		permMap[p.Code] = PermissionMapEntry{
			Roles:       roles,
			Description: p.Description,
		}
	}

	// Construir el mapa código -> centro de costo.
	ccMap := make(map[string]CostCenterMapEntry)
	for _, cc := range ccs {
		ccMap[cc.Code] = CostCenterMapEntry{
			Code:     cc.Code,
			Name:     cc.Name,
			IsActive: cc.IsActive,
		}
	}

	now := time.Now().UTC()
	generatedAt := now.Format(time.RFC3339)

	// Calcular la versión como SHA-256 del payload canónico, truncado a 4 bytes (8 hex).
	// El payload canónico ordena las claves del JSON lexicográficamente para garantizar
	// que el mismo contenido produzca siempre el mismo hash.
	payload := canonicalJSONPayload(app.Slug, generatedAt, permMap, ccMap)
	versionBytes := sha256.Sum256(payload)
	version := fmt.Sprintf("%x", versionBytes[:4]) // 8 caracteres hexadecimales

	// Firmar el payload con RSA-SHA256. Los backends consumidores pueden verificar
	// la firma usando la clave pública disponible en /.well-known/jwks.json.
	sig, err := s.tokenMgr.SignPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("authz: sign permissions map: %w", err)
	}

	return &PermissionsMapResponse{
		Application: app.Slug,
		GeneratedAt: now,
		Version:     version,
		Permissions: permMap,
		CostCenters: ccMap,
		Signature:   sig,
	}, nil
}

// getRolesForPermission devuelve los nombres de todos los roles activos que tienen
// asignado el permiso identificado por permID.
func (s *AuthzService) getRolesForPermission(ctx context.Context, permID uuid.UUID) ([]string, error) {
	const q = `
		SELECT r.name
		FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id
		WHERE rp.permission_id = $1 AND r.is_active = TRUE`
	// La query anterior es conceptual; la implementación usa el método del repositorio
	// para mantener el acceso a DB centralizado en la capa de repositorios.
	_ = q
	roles, err := s.roleRepo.GetRolesForPermission(ctx, permID)
	if err != nil {
		return nil, fmt.Errorf("authz: get roles for permission: %w", err)
	}
	return roles, nil
}

// canonicalJSONPayload serializa el payload del mapa de permisos en forma canónica:
// claves del JSON ordenadas lexicográficamente y sin espacios en blanco.
// Esta representación es determinista y se usa tanto para calcular la versión (SHA-256)
// como para firmar el mapa con RSA-SHA256.
//
// El campo "version" se deja vacío en el payload de firma para evitar circularidad
// (la versión se calcula a partir del payload).
func canonicalJSONPayload(application, generatedAt string, permissions map[string]PermissionMapEntry, costCenters map[string]CostCenterMapEntry) []byte {
	// Estructuras intermedias con tags JSON en orden lexicográfico para serialización canónica.
	type canonicalEntry struct {
		Description string   `json:"description"`
		Roles       []string `json:"roles"`
	}
	type canonicalCC struct {
		Code     string `json:"code"`
		IsActive bool   `json:"is_active"`
		Name     string `json:"name"`
	}
	type canonicalPayload struct {
		Application string                    `json:"application"`
		CostCenters map[string]canonicalCC    `json:"cost_centers"`
		GeneratedAt string                    `json:"generated_at"`
		Permissions map[string]canonicalEntry `json:"permissions"`
		Version     string                    `json:"version"` // vacío en el payload de firma
	}

	perms := make(map[string]canonicalEntry)
	for k, v := range permissions {
		// Ordenar los roles dentro de cada permiso para garantizar determinismo.
		roles := make([]string, len(v.Roles))
		copy(roles, v.Roles)
		sort.Strings(roles)
		perms[k] = canonicalEntry{Description: v.Description, Roles: roles}
	}

	ccs := make(map[string]canonicalCC)
	for k, v := range costCenters {
		ccs[k] = canonicalCC{Code: v.Code, IsActive: v.IsActive, Name: v.Name}
	}

	p := canonicalPayload{
		Application: application,
		CostCenters: ccs,
		GeneratedAt: generatedAt,
		Permissions: perms,
		Version:     "", // excluido del cálculo de versión para evitar circularidad
	}
	b, _ := json.Marshal(p)
	return b
}

// GetPermissionsMapVersion devuelve la versión actual del mapa de permisos de una
// aplicación. Primero intenta obtenerla de Redis; si no existe, construye el mapa
// completo para calcularla.
// Retorna la versión (string de 8 hex), el instante de generación y un error si falla.
func (s *AuthzService) GetPermissionsMapVersion(ctx context.Context, appSlug string) (string, time.Time, error) {
	version, err := s.authzCache.GetPermissionsMapVersion(ctx, appSlug)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("authz: get map version: %w", err)
	}
	if version == "" {
		// Sin caché: construir el mapa completo para obtener la versión.
		resp, err := s.GetPermissionsMap(ctx, appSlug)
		if err != nil {
			return "", time.Time{}, err
		}
		return resp.Version, resp.GeneratedAt, nil
	}
	return version, time.Now().UTC(), nil
}

// resolveAppID obtiene el UUID de una aplicación a partir de su slug.
// Usado internamente por Verify y GetUserPermissions para convertir el slug del JWT
// en el ID requerido por los repositorios.
func (s *AuthzService) resolveAppID(ctx context.Context, slug string) (uuid.UUID, error) {
	app, err := s.appRepo.FindBySlug(ctx, slug)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("authz: find app by slug: %w", err)
	}
	if app == nil {
		return uuid.UUID{}, fmt.Errorf("authz: app not found: %s", slug)
	}
	return app.ID, nil
}

// HasPermission verifica si el usuario tiene el permiso indicado.
// Es la interfaz usada por el middleware de autorización de Sentinel para proteger
// endpoints del panel de administración.
//
// Internamente usa GetOrBuildUserContext para aprovechar el caché Redis.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - claims: claims del JWT del usuario.
//   - permCode: código del permiso a verificar (ej: "admin.users.read").
//
// Retorna true si el usuario tiene el permiso, false en caso contrario.
func (s *AuthzService) HasPermission(ctx context.Context, claims *domain.Claims, permCode string) (bool, error) {
	appID, err := s.resolveAppID(ctx, claims.App)
	if err != nil {
		return false, err
	}
	uc, err := s.GetOrBuildUserContext(ctx, claims, appID)
	if err != nil {
		return false, err
	}
	return s.hasPermission(uc, permCode), nil
}

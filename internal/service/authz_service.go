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

// AuthzService implements authorization business logic.
type AuthzService struct {
	appRepo      *postgres.ApplicationRepository
	userRoleRepo *postgres.UserRoleRepository
	userPermRepo *postgres.UserPermissionRepository
	userCCRepo   *postgres.UserCostCenterRepository
	permRepo     *postgres.PermissionRepository
	roleRepo     *postgres.RoleRepository
	ccRepo       *postgres.CostCenterRepository
	authzCache   *redisrepo.AuthzCache
	tokenMgr     *token.Manager
	auditSvc     *AuditService
}

// NewAuthzService creates an AuthzService with all required dependencies.
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

// VerifyRequest holds the input for permission verification.
type VerifyRequest struct {
	Permission   string
	CostCenterID string
}

// VerifyResponse holds the result of permission verification.
type VerifyResponse struct {
	Allowed     bool      `json:"allowed"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Permission  string    `json:"permission"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// Verify checks if a user has a specific permission.
func (s *AuthzService) Verify(ctx context.Context, claims *domain.Claims, req VerifyRequest, ip, ua string) (*VerifyResponse, error) {
	userID, _ := uuid.Parse(claims.Sub)
	appID, err := s.resolveAppID(ctx, claims.App)
	if err != nil {
		return nil, err
	}

	// Get or build user context from cache.
	uc, err := s.GetOrBuildUserContext(ctx, claims, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get user context: %w", err)
	}

	allowed := s.hasPermission(uc, req.Permission)

	// Check cost center if specified.
	if allowed && req.CostCenterID != "" {
		allowed = s.hasCostCenter(uc, req.CostCenterID)
	}

	// Audit the authorization decision.
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

func (s *AuthzService) hasPermission(uc *redisrepo.UserContext, permCode string) bool {
	for _, p := range uc.Permissions {
		if p == permCode {
			return true
		}
	}
	return false
}

func (s *AuthzService) hasCostCenter(uc *redisrepo.UserContext, ccCode string) bool {
	for _, cc := range uc.CostCenters {
		if cc == ccCode {
			return true
		}
	}
	return false
}

// GetOrBuildUserContext retrieves the user context from cache, or builds it from DB.
func (s *AuthzService) GetOrBuildUserContext(ctx context.Context, claims *domain.Claims, appID uuid.UUID) (*redisrepo.UserContext, error) {
	// Try cache first.
	cached, err := s.authzCache.GetPermissions(ctx, claims.Jti)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Build from DB.
	uc, err := s.buildUserContext(ctx, claims, appID)
	if err != nil {
		return nil, err
	}

	// Cache with TTL matching the access token TTL (60 min).
	ttl := 60 * time.Minute
	_ = s.authzCache.SetPermissions(ctx, claims.Jti, uc, ttl)
	return uc, nil
}

// buildUserContext assembles the user context from the database.
func (s *AuthzService) buildUserContext(ctx context.Context, claims *domain.Claims, appID uuid.UUID) (*redisrepo.UserContext, error) {
	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("authz: invalid user id: %w", err)
	}

	// Get permissions from active roles (via role_permissions).
	permSet := make(map[string]struct{})

	// Role-based permissions via active user_roles.
	rolePerms, err := s.getRolePermissionsForUser(ctx, userID, appID)
	if err != nil {
		return nil, err
	}
	for _, p := range rolePerms {
		permSet[p] = struct{}{}
	}

	// Individual permissions.
	indivPerms, err := s.userPermRepo.GetActivePermissionCodesForUserApp(ctx, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get individual permissions: %w", err)
	}
	for _, p := range indivPerms {
		permSet[p] = struct{}{}
	}

	// Flatten permissions to sorted slice.
	permissions := make([]string, 0, len(permSet))
	for p := range permSet {
		permissions = append(permissions, p)
	}
	sort.Strings(permissions)

	// Cost centers.
	costCenters, err := s.userCCRepo.GetActiveCodesForUserApp(ctx, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get cost centers: %w", err)
	}

	return &redisrepo.UserContext{
		UserID:      claims.Sub,
		Application: claims.App,
		Roles:       claims.Roles,
		Permissions: permissions,
		CostCenters: costCenters,
	}, nil
}

// getRolePermissionsForUser returns all permission codes from the user's active roles.
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
	// We need to run this query directly.
	// Since we don't have a generic DB reference here, we use the role repo's internal.
	// Design: add a helper method to roleRepo or use userRoleRepo.
	// We'll use a workaround by calling through existing repos.
	_ = q // This query is conceptual; we implement via the role repo.

	// Get active roles for user+app via userRoleRepo.
	roleNames, err := s.userRoleRepo.GetActiveRoleNamesForUserApp(ctx, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("authz: get role names: %w", err)
	}
	if len(roleNames) == 0 {
		return nil, nil
	}

	// For each role, get its permissions.
	permSet := make(map[string]struct{})
	// We need role IDs. Use roleRepo to find by name+app.
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

// MePermissionsResponse holds the full user context response.
type MePermissionsResponse struct {
	UserID         string          `json:"user_id"`
	Application    string          `json:"application"`
	Roles          []string        `json:"roles"`
	Permissions    []string        `json:"permissions"`
	CostCenters    []string        `json:"cost_centers"`
	TemporaryRoles []TemporaryRole `json:"temporary_roles"`
}

// TemporaryRole is a role with a defined valid_until.
type TemporaryRole struct {
	Role       string    `json:"role"`
	ValidUntil time.Time `json:"valid_until"`
}

// GetUserPermissions builds and returns the full user permissions context.
func (s *AuthzService) GetUserPermissions(ctx context.Context, claims *domain.Claims) (*MePermissionsResponse, error) {
	appID, err := s.resolveAppID(ctx, claims.App)
	if err != nil {
		return nil, err
	}

	uc, err := s.GetOrBuildUserContext(ctx, claims, appID)
	if err != nil {
		return nil, err
	}

	// Get temporary roles.
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

// PermissionMapEntry describes a permission in the global map.
type PermissionMapEntry struct {
	Roles       []string `json:"roles"`
	Description string   `json:"description"`
}

// CostCenterMapEntry describes a cost center in the global map.
type CostCenterMapEntry struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// PermissionsMapResponse is the signed permissions map for an application.
type PermissionsMapResponse struct {
	Application string                        `json:"application"`
	GeneratedAt time.Time                     `json:"generated_at"`
	Version     string                        `json:"version"`
	Permissions map[string]PermissionMapEntry `json:"permissions"`
	CostCenters map[string]CostCenterMapEntry `json:"cost_centers"`
	Signature   string                        `json:"signature"`
}

// GetPermissionsMap builds and signs the permissions map for an application.
func (s *AuthzService) GetPermissionsMap(ctx context.Context, appSlug string) (*PermissionsMapResponse, error) {
	app, err := s.appRepo.FindBySlug(ctx, appSlug)
	if err != nil || app == nil {
		return nil, fmt.Errorf("authz: app not found: %s", appSlug)
	}

	// Try cache.
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

	// Cache the response.
	b, _ := json.Marshal(resp)
	_ = s.authzCache.SetPermissionsMap(ctx, appSlug, b, 5*time.Minute)
	_ = s.authzCache.SetPermissionsMapVersion(ctx, appSlug, resp.Version, 5*time.Minute)

	return resp, nil
}

// buildPermissionsMap assembles the permissions map from the database.
func (s *AuthzService) buildPermissionsMap(ctx context.Context, app *domain.Application) (*PermissionsMapResponse, error) {
	perms, err := s.permRepo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("authz: list permissions: %w", err)
	}

	ccs, err := s.ccRepo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("authz: list cost centers: %w", err)
	}

	// Build permission -> roles map.
	permMap := make(map[string]PermissionMapEntry)
	for _, p := range perms {
		// Find all roles that have this permission.
		roles, err := s.getRolesForPermission(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		permMap[p.Code] = PermissionMapEntry{
			Roles:       roles,
			Description: p.Description,
		}
	}

	// Build cost center map.
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

	// Compute version hash (SHA-256 of canonical JSON).
	payload := canonicalJSONPayload(app.Slug, generatedAt, permMap, ccMap)
	versionBytes := sha256.Sum256(payload)
	version := fmt.Sprintf("%x", versionBytes[:4]) // 8 hex chars

	// Sign the payload.
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

// getRolesForPermission returns the names of all active roles that have a given permission.
func (s *AuthzService) getRolesForPermission(ctx context.Context, permID uuid.UUID) ([]string, error) {
	const q = `
		SELECT r.name
		FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id
		WHERE rp.permission_id = $1 AND r.is_active = TRUE`
	// We need DB access here. Use roleRepo's DB reference.
	// We add a method GetRolesForPermission to roleRepo.
	roles, err := s.roleRepo.GetRolesForPermission(ctx, permID)
	if err != nil {
		return nil, fmt.Errorf("authz: get roles for permission: %w", err)
	}
	return roles, nil
}

// canonicalJSONPayload serializes the map payload in canonical form (sorted keys, no spaces).
func canonicalJSONPayload(application, generatedAt string, permissions map[string]PermissionMapEntry, costCenters map[string]CostCenterMapEntry) []byte {
	// Build canonical payload with sorted keys.
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
		Version     string                    `json:"version"`
	}

	perms := make(map[string]canonicalEntry)
	for k, v := range permissions {
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
		Version:     "", // Version is computed from this payload (exclude from version computation)
	}
	b, _ := json.Marshal(p)
	return b
}

// GetPermissionsMapVersion returns the lightweight version info for an app.
func (s *AuthzService) GetPermissionsMapVersion(ctx context.Context, appSlug string) (string, time.Time, error) {
	version, err := s.authzCache.GetPermissionsMapVersion(ctx, appSlug)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("authz: get map version: %w", err)
	}
	if version == "" {
		// Build map to get version.
		resp, err := s.GetPermissionsMap(ctx, appSlug)
		if err != nil {
			return "", time.Time{}, err
		}
		return resp.Version, resp.GeneratedAt, nil
	}
	return version, time.Now().UTC(), nil
}

// resolveAppID looks up an application ID from its slug.
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

// HasPermission checks if the user has a given permission (used by middleware).
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

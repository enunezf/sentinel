package handler

import (
	"crypto/rand"
	"encoding/base64"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/middleware"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	"github.com/enunezf/sentinel/internal/service"
)

var slugRegexp = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// AdminHandler handles all /admin/* endpoints.
type AdminHandler struct {
	userSvc   *service.UserService
	roleSvc   *service.RoleService
	permSvc   *service.PermissionService
	ccSvc     *service.CostCenterService
	auditRepo *postgres.AuditRepository
	appRepo   *postgres.ApplicationRepository
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(
	userSvc *service.UserService,
	roleSvc *service.RoleService,
	permSvc *service.PermissionService,
	ccSvc *service.CostCenterService,
	auditRepo *postgres.AuditRepository,
	appRepo *postgres.ApplicationRepository,
) *AdminHandler {
	return &AdminHandler{
		userSvc:   userSvc,
		roleSvc:   roleSvc,
		permSvc:   permSvc,
		ccSvc:     ccSvc,
		auditRepo: auditRepo,
		appRepo:   appRepo,
	}
}

// pagination helpers.
func parsePagination(c *fiber.Ctx) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page", "1"))
	pageSize, _ = strconv.Atoi(c.Query("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

func totalPages(total, pageSize int) int {
	if pageSize == 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func paginatedResponse(data interface{}, page, pageSize, total int) fiber.Map {
	return fiber.Map{
		"data":        data,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages(total, pageSize),
	}
}

// ---- USER ENDPOINTS ----

// ListUsers handles GET /admin/users.
func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	search := c.Query("search", "")

	var isActive *bool
	if v := c.Query("is_active", ""); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActive = &b
		}
	}

	users, total, err := h.userSvc.ListUsers(c.Context(), postgres.UserFilter{
		Search:   search,
		IsActive: isActive,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	data := make([]fiber.Map, 0, len(users))
	for _, u := range users {
		data = append(data, fiber.Map{
			"id":              u.ID,
			"username":        u.Username,
			"email":           u.Email,
			"is_active":       u.IsActive,
			"must_change_pwd": u.MustChangePwd,
			"last_login_at":   u.LastLoginAt,
			"failed_attempts": u.FailedAttempts,
			"locked_until":    u.LockedUntil,
			"created_at":      u.CreatedAt,
		})
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(data, page, pageSize, total))
}

// CreateUser handles POST /admin/users.
func (h *AdminHandler) CreateUser(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Username == "" || body.Email == "" || body.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "username, email, and password are required")
	}

	user, err := h.userSvc.CreateUser(c.Context(), service.CreateUserRequest{
		Username:  body.Username,
		Email:     body.Email,
		Password:  body.Password,
		ActorID:   actorID,
		IP:        getIP(c),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_active":       user.IsActive,
		"must_change_pwd": user.MustChangePwd,
		"created_at":      user.CreatedAt,
	})
}

// GetUser handles GET /admin/users/:id.
func (h *AdminHandler) GetUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	user, err := h.userSvc.GetUser(c.Context(), id)
	if err != nil || user == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "user not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_active":       user.IsActive,
		"must_change_pwd": user.MustChangePwd,
		"last_login_at":   user.LastLoginAt,
		"failed_attempts": user.FailedAttempts,
		"locked_until":    user.LockedUntil,
		"created_at":      user.CreatedAt,
		"updated_at":      user.UpdatedAt,
	})
}

// UpdateUser handles PUT /admin/users/:id.
func (h *AdminHandler) UpdateUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Username *string `json:"username"`
		Email    *string `json:"email"`
		IsActive *bool   `json:"is_active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	user, err := h.userSvc.UpdateUser(c.Context(), id, service.UpdateUserRequest{
		Username:  body.Username,
		Email:     body.Email,
		IsActive:  body.IsActive,
		ActorID:   actorID,
		IP:        getIP(c),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_active":       user.IsActive,
		"must_change_pwd": user.MustChangePwd,
		"updated_at":      user.UpdatedAt,
	})
}

// UnlockUser handles POST /admin/users/:id/unlock.
func (h *AdminHandler) UnlockUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.userSvc.UnlockUser(c.Context(), id, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ResetPassword handles POST /admin/users/:id/reset-password.
func (h *AdminHandler) ResetPassword(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	tempPwd, err := h.userSvc.ResetPassword(c.Context(), id, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"temporary_password": tempPwd,
	})
}

// AssignRole handles POST /admin/users/:id/roles.
func (h *AdminHandler) AssignRole(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	app := middleware.GetApp(c)
	appID := uuid.Nil
	if app != nil {
		appID = app.ID
	}

	var body struct {
		RoleID     uuid.UUID  `json:"role_id"`
		ValidFrom  *time.Time `json:"valid_from"`
		ValidUntil *time.Time `json:"valid_until"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.RoleID == uuid.Nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "role_id is required")
	}

	ur, err := h.userSvc.AssignRole(c.Context(), userID, service.AssignRoleRequest{
		RoleID:     body.RoleID,
		ValidFrom:  body.ValidFrom,
		ValidUntil: body.ValidUntil,
		ActorID:    actorID,
		AppID:      appID,
		IP:         getIP(c),
		UserAgent:  c.Get("User-Agent"),
	})
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":          ur.ID,
		"user_id":     userID,
		"role_id":     ur.RoleID,
		"role_name":   ur.RoleName,
		"valid_from":  ur.ValidFrom,
		"valid_until": ur.ValidUntil,
		"granted_by":  ur.GrantedBy,
	})
}

// RevokeRole handles DELETE /admin/users/:id/roles/:rid.
func (h *AdminHandler) RevokeRole(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}
	rid, err := uuid.Parse(c.Params("rid"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role assignment id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.userSvc.RevokeRole(c.Context(), userID, rid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// AssignPermission handles POST /admin/users/:id/permissions.
func (h *AdminHandler) AssignPermission(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	app := middleware.GetApp(c)
	appID := uuid.Nil
	if app != nil {
		appID = app.ID
	}

	var body struct {
		PermissionID uuid.UUID  `json:"permission_id"`
		ValidFrom    *time.Time `json:"valid_from"`
		ValidUntil   *time.Time `json:"valid_until"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	up, err := h.userSvc.AssignPermission(c.Context(), userID, service.AssignPermissionRequest{
		PermissionID: body.PermissionID,
		ValidFrom:    body.ValidFrom,
		ValidUntil:   body.ValidUntil,
		ActorID:      actorID,
		AppID:        appID,
		IP:           getIP(c),
		UserAgent:    c.Get("User-Agent"),
	})
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":            up.ID,
		"user_id":       userID,
		"permission_id": up.PermissionID,
		"valid_from":    up.ValidFrom,
		"valid_until":   up.ValidUntil,
		"granted_by":    up.GrantedBy,
	})
}

// RevokePermission handles DELETE /admin/users/:id/permissions/:pid.
func (h *AdminHandler) RevokePermission(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}
	pid, err := uuid.Parse(c.Params("pid"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid permission assignment id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.userSvc.RevokePermission(c.Context(), userID, pid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// AssignCostCenters handles POST /admin/users/:id/cost-centers.
func (h *AdminHandler) AssignCostCenters(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	app := middleware.GetApp(c)
	appID := uuid.Nil
	if app != nil {
		appID = app.ID
	}

	var body struct {
		CostCenterIDs []uuid.UUID `json:"cost_center_ids"`
		ValidFrom     *time.Time  `json:"valid_from"`
		ValidUntil    *time.Time  `json:"valid_until"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	if err := h.userSvc.AssignCostCenters(c.Context(), userID, service.AssignCostCentersRequest{
		CostCenterIDs: body.CostCenterIDs,
		ValidFrom:     body.ValidFrom,
		ValidUntil:    body.ValidUntil,
		ActorID:       actorID,
		AppID:         appID,
		IP:            getIP(c),
		UserAgent:     c.Get("User-Agent"),
	}); err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"assigned": len(body.CostCenterIDs)})
}

// ---- ROLE ENDPOINTS ----

// ListRoles handles GET /admin/roles.
func (h *AdminHandler) ListRoles(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	app := middleware.GetApp(c)

	filter := postgres.RoleFilter{Page: page, PageSize: pageSize}
	if app != nil {
		appID := app.ID
		filter.ApplicationID = &appID
	}

	roles, total, err := h.roleSvc.ListRoles(c.Context(), filter)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	data := make([]fiber.Map, 0, len(roles))
	for _, r := range roles {
		data = append(data, fiber.Map{
			"id":          r.ID,
			"name":        r.Name,
			"description": r.Description,
			"is_system":   r.IsSystem,
			"is_active":   r.IsActive,
			"created_at":  r.CreatedAt,
		})
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(data, page, pageSize, total))
}

// CreateRole handles POST /admin/roles.
func (h *AdminHandler) CreateRole(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "name is required")
	}

	role, err := h.roleSvc.CreateRole(c.Context(), app.ID, body.Name, body.Description, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(role)
}

// GetRole handles GET /admin/roles/:id.
func (h *AdminHandler) GetRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	role, err := h.roleSvc.GetRole(c.Context(), id)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "role not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":          role.ID,
		"name":        role.Name,
		"description": role.Description,
		"is_system":   role.IsSystem,
		"is_active":   role.IsActive,
		"permissions": role.Permissions,
		"users_count": role.UsersCount,
		"created_at":  role.CreatedAt,
		"updated_at":  role.UpdatedAt,
	})
}

// UpdateRole handles PUT /admin/roles/:id.
func (h *AdminHandler) UpdateRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	role, err := h.roleSvc.UpdateRole(c.Context(), id, body.Name, body.Description, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(role)
}

// DeleteRole handles DELETE /admin/roles/:id.
func (h *AdminHandler) DeleteRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.roleSvc.DeactivateRole(c.Context(), id, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return mapServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// AddRolePermission handles POST /admin/roles/:id/permissions.
func (h *AdminHandler) AddRolePermission(c *fiber.Ctx) error {
	roleID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		PermissionIDs []uuid.UUID `json:"permission_ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	for _, pid := range body.PermissionIDs {
		if err := h.roleSvc.AddPermissionToRole(c.Context(), roleID, pid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
			return mapServiceError(c, err)
		}
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"assigned": len(body.PermissionIDs)})
}

// RemoveRolePermission handles DELETE /admin/roles/:id/permissions/:pid.
func (h *AdminHandler) RemoveRolePermission(c *fiber.Ctx) error {
	roleID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}
	pid, err := uuid.Parse(c.Params("pid"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid permission id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.roleSvc.RemovePermissionFromRole(c.Context(), roleID, pid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return mapServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ---- PERMISSION ENDPOINTS ----

// ListPermissions handles GET /admin/permissions.
func (h *AdminHandler) ListPermissions(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	app := middleware.GetApp(c)

	filter := postgres.PermissionFilter{Page: page, PageSize: pageSize}
	if app != nil {
		appID := app.ID
		filter.ApplicationID = &appID
	}

	perms, total, err := h.permSvc.ListPermissions(c.Context(), filter)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(perms, page, pageSize, total))
}

// CreatePermission handles POST /admin/permissions.
func (h *AdminHandler) CreatePermission(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Code        string `json:"code"`
		Description string `json:"description"`
		ScopeType   string `json:"scope_type"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Code == "" || body.ScopeType == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "code and scope_type are required")
	}

	perm, err := h.permSvc.CreatePermission(c.Context(), app.ID, body.Code, body.Description, body.ScopeType, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(perm)
}

// DeletePermission handles DELETE /admin/permissions/:id.
func (h *AdminHandler) DeletePermission(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid permission id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.permSvc.DeletePermission(c.Context(), id, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return mapServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ---- COST CENTER ENDPOINTS ----

// ListCostCenters handles GET /admin/cost-centers.
func (h *AdminHandler) ListCostCenters(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	app := middleware.GetApp(c)

	filter := postgres.CCFilter{Page: page, PageSize: pageSize}
	if app != nil {
		appID := app.ID
		filter.ApplicationID = &appID
	}

	ccs, total, err := h.ccSvc.ListCostCenters(c.Context(), filter)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(ccs, page, pageSize, total))
}

// CreateCostCenter handles POST /admin/cost-centers.
func (h *AdminHandler) CreateCostCenter(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Code == "" || body.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "code and name are required")
	}

	cc, err := h.ccSvc.CreateCostCenter(c.Context(), app.ID, body.Code, body.Name, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(cc)
}

// UpdateCostCenter handles PUT /admin/cost-centers/:id.
func (h *AdminHandler) UpdateCostCenter(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid cost center id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	body.IsActive = true // default
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	cc, err := h.ccSvc.UpdateCostCenter(c.Context(), id, body.Name, body.IsActive, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(cc)
}

// ---- AUDIT LOGS ENDPOINT ----

// ListAuditLogs handles GET /admin/audit-logs.
func (h *AdminHandler) ListAuditLogs(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)

	filter := postgres.AuditFilter{
		Page:      page,
		PageSize:  pageSize,
		EventType: c.Query("event_type", ""),
	}

	if v := c.Query("user_id", ""); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.UserID = &id
		}
	}
	if v := c.Query("actor_id", ""); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.ActorID = &id
		}
	}
	if v := c.Query("application_id", ""); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.ApplicationID = &id
		}
	}
	if v := c.Query("from_date", ""); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.FromDate = &t
		}
	}
	if v := c.Query("to_date", ""); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.ToDate = &t
		}
	}
	if v := c.Query("success", ""); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			filter.Success = &b
		}
	}

	logs, total, err := h.auditRepo.List(c.Context(), filter)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(logs, page, pageSize, total))
}

// ---- APPLICATION ENDPOINTS ----

// ListApplications handles GET /admin/applications.
func (h *AdminHandler) ListApplications(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	search := c.Query("search", "")

	var isActive *bool
	if v := c.Query("is_active", ""); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActive = &b
		}
	}

	apps, total, err := h.appRepo.List(c.Context(), postgres.ApplicationFilter{
		Search:   search,
		IsActive: isActive,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	data := make([]fiber.Map, 0, len(apps))
	for _, a := range apps {
		data = append(data, fiber.Map{
			"id":         a.ID,
			"name":       a.Name,
			"slug":       a.Slug,
			"is_active":  a.IsActive,
			"is_system":  a.Slug == "system",
			"created_at": a.CreatedAt,
			"updated_at": a.UpdatedAt,
		})
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(data, page, pageSize, total))
}

// GetApplication handles GET /admin/applications/:id.
func (h *AdminHandler) GetApplication(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid application id")
	}

	app, err := h.appRepo.FindByID(c.Context(), id)
	if err != nil || app == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "application not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":         app.ID,
		"name":       app.Name,
		"slug":       app.Slug,
		"secret_key": app.SecretKey,
		"is_active":  app.IsActive,
		"is_system":  app.Slug == "system",
		"created_at": app.CreatedAt,
		"updated_at": app.UpdatedAt,
	})
}

// CreateApplication handles POST /admin/applications.
func (h *AdminHandler) CreateApplication(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Name == "" || body.Slug == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "name and slug are required")
	}
	if !slugRegexp.MatchString(body.Slug) {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "slug must be lowercase alphanumeric with hyphens (e.g. my-app)")
	}

	existing, err := h.appRepo.FindBySlug(c.Context(), body.Slug)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	if existing != nil {
		return respondError(c, fiber.StatusConflict, "CONFLICT", "an application with this slug already exists")
	}

	secretKey, err := generateAppSecretKey()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate secret key")
	}

	app := &domain.Application{
		ID:        uuid.New(),
		Name:      body.Name,
		Slug:      body.Slug,
		SecretKey: secretKey,
		IsActive:  true,
	}
	if err := h.appRepo.Create(c.Context(), app); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":         app.ID,
		"name":       app.Name,
		"slug":       app.Slug,
		"secret_key": app.SecretKey,
		"is_active":  app.IsActive,
		"is_system":  false,
		"created_at": app.CreatedAt,
	})
}

// UpdateApplication handles PUT /admin/applications/:id.
func (h *AdminHandler) UpdateApplication(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid application id")
	}

	existing, err := h.appRepo.FindByID(c.Context(), id)
	if err != nil || existing == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "application not found")
	}
	if existing.Slug == "system" {
		return respondError(c, fiber.StatusForbidden, "FORBIDDEN", "the system application cannot be modified")
	}

	var body struct {
		Name     string `json:"name"`
		IsActive *bool  `json:"is_active"`
	}
	body.IsActive = &existing.IsActive
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Name == "" {
		body.Name = existing.Name
	}

	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	updated, err := h.appRepo.Update(c.Context(), id, body.Name, isActive)
	if err != nil || updated == nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to update application")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":         updated.ID,
		"name":       updated.Name,
		"slug":       updated.Slug,
		"is_active":  updated.IsActive,
		"is_system":  updated.Slug == "system",
		"updated_at": updated.UpdatedAt,
	})
}

// RotateApplicationKey handles POST /admin/applications/:id/rotate-key.
func (h *AdminHandler) RotateApplicationKey(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid application id")
	}

	existing, err := h.appRepo.FindByID(c.Context(), id)
	if err != nil || existing == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "application not found")
	}
	if existing.Slug == "system" {
		return respondError(c, fiber.StatusForbidden, "FORBIDDEN", "the system application key cannot be rotated via API")
	}

	newKey, err := generateAppSecretKey()
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate secret key")
	}

	if err := h.appRepo.RotateSecretKey(c.Context(), id, newKey); err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"secret_key": newKey,
	})
}

func generateAppSecretKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// mapServiceError maps service errors to HTTP responses.
func mapServiceError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case isPasswordPolicyError(err):
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", msg)
	default:
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", msg)
	}
}

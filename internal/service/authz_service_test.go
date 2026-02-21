package service_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
	"github.com/enunezf/sentinel/internal/service"
	"github.com/enunezf/sentinel/internal/token"
)

// ---- HasPermission tests ----

// buildUC builds a UserContext with the given roles, permissions and cost centers.
func buildUC(roles, permissions, costCenters []string) *redisrepo.UserContext {
	return &redisrepo.UserContext{
		UserID:      testUserID.String(),
		Application: "test-app",
		Roles:       roles,
		Permissions: permissions,
		CostCenters: costCenters,
	}
}

// TestHasPermission_ByRole: user with role that has permission -> PERMITTED.
func TestHasPermission_ByRole(t *testing.T) {
	uc := buildUC([]string{"chef"}, []string{"inventory.stock.read"}, nil)
	assert.True(t, service.CheckHasPermission(uc, "inventory.stock.read", ""),
		"user with permission from role must be PERMITTED")
}

// TestHasPermission_NoRole: user without required permission -> DENIED.
func TestHasPermission_NoRole(t *testing.T) {
	uc := buildUC([]string{"chef"}, []string{"inventory.stock.read"}, nil)
	assert.False(t, service.CheckHasPermission(uc, "finance.ceco.write", ""),
		"user missing permission must be DENIED")
}

// TestHasPermission_ExtraPermission: individual permission (not from role) -> PERMITTED.
func TestHasPermission_ExtraPermission(t *testing.T) {
	uc := buildUC(nil, []string{"reports.special.export"}, nil)
	assert.True(t, service.CheckHasPermission(uc, "reports.special.export", ""),
		"user with extra individual permission must be PERMITTED")
}

// TestHasPermission_CostCenter_Valid: permission OK + matching CeCo -> PERMITTED.
func TestHasPermission_CostCenter_Valid(t *testing.T) {
	uc := buildUC([]string{"chef"}, []string{"inventory.stock.read"}, []string{"CC001", "CC002"})
	assert.True(t, service.CheckHasPermission(uc, "inventory.stock.read", "CC001"),
		"user with permission and matching CeCo must be PERMITTED")
}

// TestHasPermission_CostCenter_Invalid: permission OK + unassigned CeCo -> DENIED.
func TestHasPermission_CostCenter_Invalid(t *testing.T) {
	uc := buildUC([]string{"chef"}, []string{"inventory.stock.read"}, []string{"CC001"})
	assert.False(t, service.CheckHasPermission(uc, "inventory.stock.read", "CC999"),
		"user missing required CeCo must be DENIED")
}

// TestHasPermission_CostCenter_NotRequired: permission OK, no CeCo required -> PERMITTED.
func TestHasPermission_CostCenter_NotRequired(t *testing.T) {
	uc := buildUC([]string{"chef"}, []string{"inventory.stock.read"}, nil)
	assert.True(t, service.CheckHasPermission(uc, "inventory.stock.read", ""),
		"user with permission and no CeCo requirement must be PERMITTED")
}

// TestHasPermission_TemporalRole_Active: permissions from an active temporal role -> PERMITTED.
func TestHasPermission_TemporalRole_Active(t *testing.T) {
	// An active temporal role contributes its permissions to the context.
	uc := buildUC([]string{"bodeguero-temporal"}, []string{"inventory.stock.write"}, nil)
	assert.True(t, service.CheckHasPermission(uc, "inventory.stock.write", ""),
		"active temporal role permissions must be PERMITTED")
}

// TestHasPermission_TemporalRole_Expired: expired role not in context -> DENIED.
func TestHasPermission_TemporalRole_Expired(t *testing.T) {
	// An expired role's permissions are absent from the context.
	uc := buildUC([]string{}, []string{}, nil)
	assert.False(t, service.CheckHasPermission(uc, "inventory.stock.write", ""),
		"expired temporal role must be DENIED")
}

// TestHasPermission_IndividualPermission_Expired: expired individual permission -> DENIED.
func TestHasPermission_IndividualPermission_Expired(t *testing.T) {
	uc := buildUC(nil, []string{}, nil)
	assert.False(t, service.CheckHasPermission(uc, "reports.special.export", ""),
		"expired individual permission must be DENIED")
}

// TestHasPermission_UnknownPermission: unknown permission code -> DENIED.
func TestHasPermission_UnknownPermission(t *testing.T) {
	uc := buildUC([]string{"admin"}, []string{"admin.system.manage"}, nil)
	assert.False(t, service.CheckHasPermission(uc, "xxx.yyy.zzz", ""),
		"unknown permission not in context must be DENIED")
}

// TestGetUserPermissions_Union: role + individual permissions union without duplicates.
func TestGetUserPermissions_Union(t *testing.T) {
	rolePerms := []string{"inventory.stock.read", "inventory.stock.write"}
	individualPerms := []string{"inventory.stock.read", "reports.special.export"} // duplicate "read"

	merged := service.MergePermissions(rolePerms, individualPerms)

	assert.Len(t, merged, 3, "union of permissions must not contain duplicates")
	assert.Contains(t, merged, "inventory.stock.read")
	assert.Contains(t, merged, "inventory.stock.write")
	assert.Contains(t, merged, "reports.special.export")
}

// TestGetUserPermissions_ExcludesInactive: inactive role permissions excluded.
func TestGetUserPermissions_ExcludesInactive(t *testing.T) {
	// Simulates that an inactive role's permissions are not included at build time.
	activePerms := []string{"inventory.stock.read"}
	merged := service.MergePermissions(activePerms, []string{})
	assert.NotContains(t, merged, "finance.ceco.write",
		"permission from inactive role must be excluded")
}

// TestPermissionsMap_Signature: signature is verifiable with the RSA public key.
func TestPermissionsMap_Signature(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	mgr := token.NewManagerFromKey(privKey)

	permMap := map[string]service.PermissionMapEntry{
		"inventory.stock.read": {Roles: []string{"chef"}, Description: "Ver stock"},
	}
	ccMap := map[string]service.CostCenterMapEntry{
		"CC001": {Code: "CC001", Name: "Casino Central", IsActive: true},
	}

	payload := service.CanonicalJSONPayload("test-app", "2026-02-21T00:00:00Z", permMap, ccMap)
	sig, err := mgr.SignPayload(payload)
	require.NoError(t, err)

	sigBytes, err := base64.RawURLEncoding.DecodeString(sig)
	require.NoError(t, err)
	digest := sha256.Sum256(payload)

	err = rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.SHA256, digest[:], sigBytes)
	assert.NoError(t, err, "generated signature must be verifiable with public key")
}

// TestPermissionsMap_InvalidSignature: modifying the payload invalidates the signature.
func TestPermissionsMap_InvalidSignature(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	mgr := token.NewManagerFromKey(privKey)

	permMap := map[string]service.PermissionMapEntry{
		"inventory.stock.read": {Roles: []string{"chef"}, Description: "Ver stock"},
	}
	ccMap := map[string]service.CostCenterMapEntry{}

	payload := service.CanonicalJSONPayload("test-app", "2026-02-21T00:00:00Z", permMap, ccMap)
	sig, err := mgr.SignPayload(payload)
	require.NoError(t, err)

	// Tamper with the payload.
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(payload, &m))
	m["application"] = "tampered-app"
	tamperedPayload, _ := json.Marshal(m)

	sigBytes, err := base64.RawURLEncoding.DecodeString(sig)
	require.NoError(t, err)

	digest := sha256.Sum256(tamperedPayload)
	err = rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.SHA256, digest[:], sigBytes)
	assert.Error(t, err, "signature on tampered payload must not verify")
}

// TestPermissionsMap_VersionChanges: modifying role assignment changes the version hash.
func TestPermissionsMap_VersionChanges(t *testing.T) {
	ccMap := map[string]service.CostCenterMapEntry{}

	permMapV1 := map[string]service.PermissionMapEntry{
		"inventory.stock.read": {Roles: []string{"chef"}, Description: "Ver stock"},
	}
	payloadV1 := service.CanonicalJSONPayload("test-app", "2026-02-21T00:00:00Z", permMapV1, ccMap)
	versionV1 := permissionsVersion(payloadV1)

	permMapV2 := map[string]service.PermissionMapEntry{
		"inventory.stock.read": {Roles: []string{"chef", "admin"}, Description: "Ver stock"},
	}
	payloadV2 := service.CanonicalJSONPayload("test-app", "2026-02-21T00:00:00Z", permMapV2, ccMap)
	versionV2 := permissionsVersion(payloadV2)

	assert.NotEqual(t, versionV1, versionV2, "adding a role must change the permissions map version")
}

// permissionsVersion computes the short 8-char hex version as the service does.
func permissionsVersion(payload []byte) string {
	h := sha256.Sum256(payload)
	return fmt.Sprintf("%x", h[:4])
}

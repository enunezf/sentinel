// authz_helpers.go expone la lógica interna de autorización como funciones exportadas
// para que puedan ser probadas desde el paquete service_test sin acceso a la base de datos.
// Estas funciones replican el comportamiento de los métodos privados de AuthzService.
package service

import (
	"sort"

	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// CheckHasPermission evalúa si un contexto de usuario contiene el código de permiso
// indicado, con la restricción opcional de centro de costo.
//
// Es la versión exportada y testeable de la combinación privada
// hasPermission + hasCostCenter de AuthzService.
//
// Parámetros:
//   - uc: contexto de usuario obtenido desde Redis o construido desde PostgreSQL.
//   - permCode: código del permiso a verificar (ej: "admin.users.read").
//   - costCenterCode: código del centro de costo requerido; si es vacío, no se aplica
//     la restricción de centro de costo.
//
// Retorna true solo si el permiso existe y, cuando se especifica, el centro de costo
// también está asignado al usuario.
func CheckHasPermission(uc *redisrepo.UserContext, permCode, costCenterCode string) bool {
	hasPermission := false
	for _, p := range uc.Permissions {
		if p == permCode {
			hasPermission = true
			break
		}
	}
	if !hasPermission {
		return false
	}
	// Si no se requiere un centro de costo específico, el permiso es suficiente.
	if costCenterCode == "" {
		return true
	}
	// Verificar que el centro de costo esté asignado al usuario.
	for _, cc := range uc.CostCenters {
		if cc == costCenterCode {
			return true
		}
	}
	return false
}

// MergePermissions devuelve la unión sin duplicados de dos slices de permisos,
// ordenada lexicográficamente. Replica la lógica de unión que buildUserContext
// aplica al combinar permisos de roles con permisos individuales.
//
// Se usa en tests para verificar que la unión sea correcta sin necesidad de
// una base de datos.
//
// Parámetros:
//   - rolePerms: permisos heredados de roles activos.
//   - individualPerms: permisos asignados directamente al usuario.
//
// Retorna un slice ordenado con todos los permisos únicos de ambas fuentes.
func MergePermissions(rolePerms, individualPerms []string) []string {
	set := make(map[string]struct{})
	for _, p := range rolePerms {
		set[p] = struct{}{}
	}
	for _, p := range individualPerms {
		set[p] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for p := range set {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

// CanonicalJSONPayload es la versión exportada de canonicalJSONPayload, usada en
// authz_service_test.go para verificar que la lógica de firma y cálculo de versión
// sea correcta. Recibe exactamente los mismos parámetros que la función interna.
//
// Parámetros:
//   - application: slug de la aplicación.
//   - generatedAt: instante de generación en formato RFC3339.
//   - permissions: mapa de permisos con sus roles y descripción.
//   - costCenters: mapa de centros de costo.
//
// Retorna los bytes del JSON canónico (claves ordenadas, sin espacios).
func CanonicalJSONPayload(application, generatedAt string, permissions map[string]PermissionMapEntry, costCenters map[string]CostCenterMapEntry) []byte {
	return canonicalJSONPayload(application, generatedAt, permissions, costCenters)
}

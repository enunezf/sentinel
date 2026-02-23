import { useEffect, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { ArrowLeft, Plus, Trash2, Users, Shield, ClipboardList } from 'lucide-react'
import { rolesApi } from '@/api/roles'
import { permissionsApi } from '@/api/permissions'
import { auditApi } from '@/api/audit'
import type { Role, Permission, AuditLog } from '@/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ConfirmDialog } from '@/components/shared/ConfirmDialog'
import { toast } from '@/hooks/useToast'
import { formatDate, formatDateRelative } from '@/lib/utils'
import { cn } from '@/lib/utils'

// ─── Tabs ──────────────────────────────────────────────────────────────────

type TabId = 'permissions' | 'users' | 'audit'

const tabs: { id: TabId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'permissions', label: 'Permisos', icon: Shield },
  { id: 'users', label: 'Usuarios', icon: Users },
  { id: 'audit', label: 'Auditoría', icon: ClipboardList },
]

// ─── Audit tab ──────────────────────────────────────────────────────────────

const roleAuditEvents = new Set([
  'ROLE_CREATED', 'ROLE_UPDATED', 'ROLE_DELETED',
  'ROLE_PERMISSION_ASSIGNED', 'ROLE_PERMISSION_REVOKED',
  'USER_ROLE_ASSIGNED', 'USER_ROLE_REVOKED',
])

const auditVariants: Record<string, 'success' | 'warning' | 'secondary' | 'destructive'> = {
  ROLE_CREATED: 'success',
  ROLE_UPDATED: 'secondary',
  ROLE_DELETED: 'destructive',
  ROLE_PERMISSION_ASSIGNED: 'success',
  ROLE_PERMISSION_REVOKED: 'warning',
  USER_ROLE_ASSIGNED: 'success',
  USER_ROLE_REVOKED: 'warning',
}

function AuditTab() {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    setIsLoading(true)
    // Fetch recent audit logs and filter for role-related events
    auditApi
      .list({ page: 1, page_size: 50 })
      .then((r) => setLogs(r.data.filter((l) => roleAuditEvents.has(l.event_type))))
      .catch(() => toast({ title: 'Error al cargar auditoría', variant: 'destructive' }))
      .finally(() => setIsLoading(false))
  }, [])

  if (isLoading) {
    return (
      <div className="p-5 space-y-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-10 animate-pulse rounded-md" style={{ backgroundColor: '#F3F4F6' }} />
        ))}
      </div>
    )
  }

  if (logs.length === 0) {
    return (
      <div className="py-12 text-center text-sm" style={{ color: '#9CA3AF' }}>
        No hay eventos de auditoría recientes para roles.
      </div>
    )
  }

  return (
    <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
      {logs.map((log) => (
        <div key={log.id} className="px-5 py-3 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <Badge
              variant={auditVariants[log.event_type] ?? 'secondary'}
              className="text-xs shrink-0"
            >
              {log.event_type}
            </Badge>
            {log.ip_address && (
              <span className="text-xs font-mono hidden sm:block" style={{ color: '#9CA3AF' }}>
                {log.ip_address}
              </span>
            )}
          </div>
          <span className="text-xs shrink-0" style={{ color: '#9CA3AF' }}>
            {formatDateRelative(log.created_at)}
          </span>
        </div>
      ))}
    </div>
  )
}

// ─── Main page ──────────────────────────────────────────────────────────────

export function RoleDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [role, setRole] = useState<Role | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<TabId>('permissions')

  // Available (unassigned) permissions
  const [availablePermissions, setAvailablePermissions] = useState<Permission[]>([])
  const [selectedPermIds, setSelectedPermIds] = useState<string[]>([])
  const [isAssigning, setIsAssigning] = useState(false)
  const [showAddPerms, setShowAddPerms] = useState(false)

  // Revoke confirm
  const [confirmRevoke, setConfirmRevoke] = useState<{ id: string; code: string } | null>(null)
  const [isRevoking, setIsRevoking] = useState(false)

  const loadRole = async () => {
    if (!id) return
    setIsLoading(true)
    try {
      const r = await rolesApi.get(id)
      setRole(r)
    } catch {
      toast({ title: 'Error al cargar el rol', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    void loadRole()
    void permissionsApi.list({ page_size: 100 }).then((r) => setAvailablePermissions(r.data))
  }, [id])

  const handleAssignPermissions = async () => {
    if (!id || selectedPermIds.length === 0) return
    setIsAssigning(true)
    try {
      await rolesApi.assignPermissions(id, { permission_ids: selectedPermIds })
      toast({
        title: 'Permisos asignados',
        description: `${selectedPermIds.length} permiso(s) asignado(s) al rol.`,
      })
      setSelectedPermIds([])
      setShowAddPerms(false)
      void loadRole()
    } catch {
      toast({ title: 'Error al asignar permisos', variant: 'destructive' })
    } finally {
      setIsAssigning(false)
    }
  }

  const handleRevokePermission = async () => {
    if (!id || !confirmRevoke) return
    setIsRevoking(true)
    try {
      await rolesApi.revokePermission(id, confirmRevoke.id)
      toast({ title: 'Permiso removido del rol' })
      setConfirmRevoke(null)
      void loadRole()
    } catch {
      toast({ title: 'Error al remover permiso', variant: 'destructive' })
    } finally {
      setIsRevoking(false)
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24" style={{ color: '#9CA3AF' }}>
        <div
          className="h-6 w-6 animate-spin rounded-full border-2 border-t-transparent mr-3"
          style={{ borderColor: '#004899', borderTopColor: 'transparent' }}
        />
        Cargando rol...
      </div>
    )
  }

  if (!role) {
    return (
      <div className="text-center py-24" style={{ color: '#9CA3AF' }}>
        <p>Rol no encontrado.</p>
        <Button variant="outline" onClick={() => navigate('/roles')} className="mt-4">
          Volver a roles
        </Button>
      </div>
    )
  }

  const assignedIds = new Set(role.permissions?.map((p) => p.id) ?? [])
  const unassigned = availablePermissions.filter((p) => !assignedIds.has(p.id))

  return (
    <div className="space-y-6">
      {/* Breadcrumb + back */}
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" onClick={() => navigate('/roles')} aria-label="Volver">
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <span className="text-sm" style={{ color: '#6B7280' }}>Roles</span>
        <span className="text-sm" style={{ color: '#9CA3AF' }}>/</span>
        <span className="text-sm font-medium" style={{ color: '#1A1A2E' }}>{role.name}</span>
      </div>

      {/* Header card */}
      <div className="bg-white rounded-xl p-5" style={{ border: '1px solid #E0E5EC' }}>
        <div className="flex items-start gap-4">
          <div
            className="h-12 w-12 rounded-lg flex items-center justify-center shrink-0 text-white text-lg font-bold"
            style={{ backgroundColor: '#7C3AED' }}
          >
            {role.name.charAt(0).toUpperCase()}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-xl font-bold" style={{ color: '#1A1A2E' }}>{role.name}</h1>
              {role.is_system && <Badge variant="system">Sistema</Badge>}
              <StatusBadge active={role.is_active} />
            </div>
            {role.description && (
              <p className="text-sm mt-0.5" style={{ color: '#6B7280' }}>{role.description}</p>
            )}
          </div>
        </div>

        {/* Stats */}
        <div
          className="grid grid-cols-3 gap-4 mt-5 pt-5"
          style={{ borderTop: '1px solid #F3F4F6' }}
        >
          <div>
            <p className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>
              Permisos
            </p>
            <p className="text-2xl font-bold" style={{ color: '#1A1A2E' }}>
              {role.permissions?.length ?? role.permissions_count ?? 0}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>
              Usuarios
            </p>
            <p className="text-2xl font-bold" style={{ color: '#1A1A2E' }}>
              {role.users_count ?? 0}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>
              Creado
            </p>
            <p className="text-sm" style={{ color: '#374151' }}>{formatDate(role.created_at)}</p>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-white rounded-xl overflow-hidden" style={{ border: '1px solid #E0E5EC' }}>
        {/* Tab bar */}
        <div
          className="flex"
          style={{ borderBottom: '1px solid #E0E5EC' }}
          role="tablist"
        >
          {tabs.map(({ id: tabId, label, icon: Icon }) => (
            <button
              key={tabId}
              role="tab"
              aria-selected={activeTab === tabId}
              onClick={() => setActiveTab(tabId)}
              className={cn(
                'flex items-center gap-2 px-5 py-3.5 text-sm font-medium whitespace-nowrap transition-colors border-b-2 -mb-px',
                activeTab === tabId
                  ? 'border-sodexo-blue text-sodexo-blue'
                  : 'border-transparent text-gray-500 hover:text-gray-800'
              )}
            >
              <Icon className="h-3.5 w-3.5" aria-hidden="true" />
              {label}
            </button>
          ))}
        </div>

        {/* ── Permissions tab ─────────────────────────────────────── */}
        {activeTab === 'permissions' && (
          <div>
            {/* Header row */}
            <div
              className="flex items-center justify-between px-5 py-3"
              style={{ borderBottom: '1px solid #F3F4F6' }}
            >
              <p className="text-sm font-medium" style={{ color: '#6B7280' }}>
                {role.permissions?.length ?? 0} permiso(s) asignado(s)
              </p>
              {!role.is_system && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setShowAddPerms((v) => !v)}
                  className="gap-1.5 text-xs"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Agregar permisos
                </Button>
              )}
            </div>

            {/* Add permissions panel */}
            {showAddPerms && !role.is_system && (
              <div
                className="px-5 py-4 space-y-3"
                style={{ backgroundColor: '#F9FAFB', borderBottom: '1px solid #E0E5EC' }}
              >
                <p className="text-xs font-medium" style={{ color: '#374151' }}>
                  Seleccione permisos a agregar:
                </p>
                <div className="max-h-48 overflow-y-auto space-y-1.5">
                  {unassigned.length === 0 ? (
                    <p className="text-xs" style={{ color: '#9CA3AF' }}>
                      Todos los permisos ya están asignados.
                    </p>
                  ) : (
                    unassigned.map((p) => (
                      <label key={p.id} className="flex items-center gap-2 text-xs cursor-pointer py-0.5">
                        <input
                          type="checkbox"
                          checked={selectedPermIds.includes(p.id)}
                          onChange={() =>
                            setSelectedPermIds((prev) =>
                              prev.includes(p.id)
                                ? prev.filter((x) => x !== p.id)
                                : [...prev, p.id]
                            )
                          }
                          className="h-3.5 w-3.5 rounded border-gray-300"
                          style={{ accentColor: '#004899' }}
                        />
                        <span className="font-mono" style={{ color: '#374151' }}>{p.code}</span>
                        <Badge variant="secondary" className="text-xs ml-auto shrink-0">
                          {p.scope_type}
                        </Badge>
                      </label>
                    ))
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    onClick={handleAssignPermissions}
                    disabled={selectedPermIds.length === 0 || isAssigning}
                  >
                    {isAssigning
                      ? 'Asignando...'
                      : `Agregar ${selectedPermIds.length > 0 ? selectedPermIds.length : ''} permiso(s)`}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => { setShowAddPerms(false); setSelectedPermIds([]) }}
                  >
                    Cancelar
                  </Button>
                </div>
              </div>
            )}

            {/* Permissions list */}
            <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
              {role.permissions && role.permissions.length > 0 ? (
                role.permissions.map((perm) => (
                  <div key={perm.id} className="px-5 py-3 flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="font-mono text-sm font-medium" style={{ color: '#1A1A2E' }}>
                        {perm.code}
                      </p>
                      {perm.description && (
                        <p className="text-xs" style={{ color: '#6B7280' }}>{perm.description}</p>
                      )}
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <Badge variant="secondary" className="text-xs">{perm.scope_type}</Badge>
                      {!role.is_system && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => setConfirmRevoke({ id: perm.id, code: perm.code })}
                          aria-label={`Remover permiso ${perm.code}`}
                        >
                          <Trash2 className="h-4 w-4 text-red-500" />
                        </Button>
                      )}
                    </div>
                  </div>
                ))
              ) : (
                <div className="py-10 text-center text-sm" style={{ color: '#9CA3AF' }}>
                  Sin permisos asignados.
                </div>
              )}
            </div>
          </div>
        )}

        {/* ── Users tab ───────────────────────────────────────────── */}
        {activeTab === 'users' && (
          <div className="p-6 text-center space-y-4">
            <div
              className="inline-flex items-center justify-center h-14 w-14 rounded-full mx-auto"
              style={{ backgroundColor: '#EFF6FF' }}
            >
              <Users className="h-6 w-6" style={{ color: '#004899' }} aria-hidden="true" />
            </div>
            <div>
              <p className="text-3xl font-bold" style={{ color: '#1A1A2E' }}>
                {role.users_count ?? 0}
              </p>
              <p className="text-sm mt-1" style={{ color: '#6B7280' }}>
                usuario(s) con el rol <strong>{role.name}</strong>
              </p>
            </div>
            <Link
              to="/users"
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-colors"
              style={{ backgroundColor: '#EFF6FF', color: '#004899' }}
              onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#DBEAFE')}
              onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#EFF6FF')}
            >
              Ver todos los usuarios →
            </Link>
          </div>
        )}

        {/* ── Audit tab ────────────────────────────────────────────── */}
        {activeTab === 'audit' && <AuditTab />}
      </div>

      {confirmRevoke && (
        <ConfirmDialog
          open
          onOpenChange={(open) => { if (!open) setConfirmRevoke(null) }}
          title="Remover permiso del rol"
          description={`¿Remover el permiso "${confirmRevoke.code}" del rol "${role.name}"?`}
          confirmLabel="Remover"
          variant="destructive"
          onConfirm={handleRevokePermission}
          isLoading={isRevoking}
        />
      )}
    </div>
  )
}

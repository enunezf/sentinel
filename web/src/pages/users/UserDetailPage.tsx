import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Shield, Key, Building2, ClipboardList, Plus, Trash2, Lock } from 'lucide-react'
import { usersApi } from '@/api/users'
import { auditApi } from '@/api/audit'
import type { User, AuditLog } from '@/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ConfirmDialog } from '@/components/shared/ConfirmDialog'
import { AsignarRolModal } from '@/components/users/AsignarRolModal'
import { AsignarPermisoModal } from '@/components/users/AsignarPermisoModal'
import { AsignarCeCosModal } from '@/components/users/AsignarCeCosModal'
import { toast } from '@/hooks/useToast'
import { formatDate, formatDateRelative } from '@/lib/utils'
import { cn } from '@/lib/utils'

// ─── Tab types ──────────────────────────────────────────────────────────────

type TabId = 'roles' | 'permissions' | 'cost_centers' | 'audit'

const tabs: { id: TabId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'roles', label: 'Roles', icon: Shield },
  { id: 'permissions', label: 'Permisos especiales', icon: Key },
  { id: 'cost_centers', label: 'Centros de costo', icon: Building2 },
  { id: 'audit', label: 'Auditoría', icon: ClipboardList },
]

// ─── Audit tab ──────────────────────────────────────────────────────────────

const auditVariants: Record<string, 'success' | 'warning' | 'secondary' | 'destructive'> = {
  AUTH_LOGIN_SUCCESS: 'success',
  AUTH_LOGIN_FAILED: 'warning',
  AUTH_ACCOUNT_LOCKED: 'destructive',
  AUTH_LOGOUT: 'secondary',
  AUTH_PASSWORD_CHANGED: 'success',
  USER_ROLE_ASSIGNED: 'success',
  USER_ROLE_REVOKED: 'warning',
  USER_PERMISSION_ASSIGNED: 'success',
  USER_PERMISSION_REVOKED: 'warning',
}

function AuditTab({ userId }: { userId: string }) {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    setIsLoading(true)
    auditApi
      .list({ user_id: userId, page: 1, page_size: 20 })
      .then((r) => setLogs(r.data))
      .catch(() => toast({ title: 'Error al cargar auditoría', variant: 'destructive' }))
      .finally(() => setIsLoading(false))
  }, [userId])

  if (isLoading) {
    return (
      <div className="p-5 space-y-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="h-10 animate-pulse rounded-md" style={{ backgroundColor: '#F3F4F6' }} />
        ))}
      </div>
    )
  }

  if (logs.length === 0) {
    return (
      <div className="py-12 text-center text-sm" style={{ color: '#9CA3AF' }}>
        No hay eventos de auditoría para este usuario.
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

export function UserDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<TabId>('roles')

  // Assignment modals
  const [showRolModal, setShowRolModal] = useState(false)
  const [showPermisoModal, setShowPermisoModal] = useState(false)
  const [showCeCosModal, setShowCeCosModal] = useState(false)

  // Revoke confirm
  const [confirmRevoke, setConfirmRevoke] = useState<{
    type: 'role' | 'permission'
    id: string
    label: string
  } | null>(null)
  const [isRevoking, setIsRevoking] = useState(false)

  const loadUser = async () => {
    if (!id) return
    setIsLoading(true)
    try {
      setUser(await usersApi.get(id))
    } catch {
      toast({ title: 'Error al cargar el usuario', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => { void loadUser() }, [id])

  const handleRevoke = async () => {
    if (!id || !confirmRevoke) return
    setIsRevoking(true)
    try {
      if (confirmRevoke.type === 'role') {
        await usersApi.revokeRole(id, confirmRevoke.id)
        toast({ title: 'Rol revocado' })
      } else {
        await usersApi.revokePermission(id, confirmRevoke.id)
        toast({ title: 'Permiso revocado' })
      }
      setConfirmRevoke(null)
      void loadUser()
    } catch {
      toast({ title: 'Error al revocar', variant: 'destructive' })
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
        Cargando usuario...
      </div>
    )
  }

  if (!user) {
    return (
      <div className="text-center py-24" style={{ color: '#9CA3AF' }}>
        <p>Usuario no encontrado.</p>
        <Button variant="outline" onClick={() => navigate('/users')} className="mt-4">
          Volver a usuarios
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Back */}
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" onClick={() => navigate('/users')} aria-label="Volver">
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <span className="text-sm" style={{ color: '#6B7280' }}>Usuarios</span>
        <span className="text-sm" style={{ color: '#9CA3AF' }}>/</span>
        <span className="text-sm font-medium" style={{ color: '#1A1A2E' }}>@{user.username}</span>
      </div>

      {/* Header card */}
      <div className="bg-white rounded-xl p-5" style={{ border: '1px solid #E0E5EC' }}>
        <div className="flex items-start gap-4">
          {/* Avatar */}
          <div
            className="h-14 w-14 rounded-full flex items-center justify-center shrink-0 text-white text-xl font-bold"
            style={{ backgroundColor: '#004899' }}
          >
            {user.username.charAt(0).toUpperCase()}
          </div>

          {/* Main info */}
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-xl font-bold" style={{ color: '#1A1A2E' }}>@{user.username}</h1>
              <StatusBadge active={user.is_active} />
              {user.locked_until && (
                <Badge variant="destructive" className="flex items-center gap-1">
                  <Lock className="h-3 w-3" aria-hidden="true" />
                  Bloqueado
                </Badge>
              )}
              {user.must_change_pwd && (
                <Badge variant="warning">Cambio de contraseña requerido</Badge>
              )}
            </div>
            <p className="text-sm mt-0.5" style={{ color: '#6B7280' }}>{user.email}</p>
          </div>
        </div>

        {/* Stats row */}
        <div
          className="grid grid-cols-2 sm:grid-cols-4 gap-4 mt-5 pt-5"
          style={{ borderTop: '1px solid #F3F4F6' }}
        >
          <Stat label="Último acceso" value={user.last_login_at ? formatDateRelative(user.last_login_at) : 'Nunca'} />
          <Stat
            label="Intentos fallidos"
            value={String(user.failed_attempts)}
            valueColor={user.failed_attempts > 0 ? '#D97706' : undefined}
          />
          <Stat label="Roles asignados" value={String(user.roles?.length ?? 0)} />
          <Stat label="Creado" value={formatDate(user.created_at)} />
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-white rounded-xl overflow-hidden" style={{ border: '1px solid #E0E5EC' }}>
        {/* Tab bar */}
        <div
          className="flex overflow-x-auto"
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
                'flex items-center gap-2 px-4 py-3.5 text-sm font-medium whitespace-nowrap transition-colors border-b-2 -mb-px',
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

        {/* ── Roles tab ─────────────────────────────────── */}
        {activeTab === 'roles' && (
          <div>
            {/* Header row */}
            <div
              className="flex items-center justify-between px-5 py-3"
              style={{ borderBottom: '1px solid #F3F4F6' }}
            >
              <p className="text-sm font-medium" style={{ color: '#6B7280' }}>
                {user.roles?.length ?? 0} rol(es) asignado(s)
              </p>
              <Button size="sm" variant="outline" onClick={() => setShowRolModal(true)} className="gap-1.5 text-xs">
                <Plus className="h-3.5 w-3.5" />
                Asignar rol
              </Button>
            </div>
            <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
              {user.roles && user.roles.length > 0 ? (
                user.roles.map((role) => {
                  const isTemporal = !!role.valid_until
                  return (
                    <div key={role.id} className="px-5 py-3 flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-sm" style={{ color: '#1A1A2E' }}>
                            {role.name}
                          </span>
                          {isTemporal && (
                            <Badge variant="temporal" className="text-xs">Temporal</Badge>
                          )}
                          {!role.is_active && (
                            <Badge variant="secondary" className="text-xs">Inactivo</Badge>
                          )}
                        </div>
                        <p className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
                          Desde {formatDate(role.valid_from)}
                          {role.valid_until ? ` · Expira ${formatDateRelative(role.valid_until)}` : ' · Sin expiración'}
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setConfirmRevoke({ type: 'role', id: role.id, label: role.name })}
                        aria-label={`Revocar rol ${role.name}`}
                      >
                        <Trash2 className="h-4 w-4 text-red-500" />
                      </Button>
                    </div>
                  )
                })
              ) : (
                <div className="py-10 text-center text-sm" style={{ color: '#9CA3AF' }}>
                  Sin roles asignados.
                </div>
              )}
            </div>
          </div>
        )}

        {/* ── Permissions tab ───────────────────────────── */}
        {activeTab === 'permissions' && (
          <div>
            <div
              className="flex items-center justify-between px-5 py-3"
              style={{ borderBottom: '1px solid #F3F4F6' }}
            >
              <div>
                <p className="text-sm font-medium" style={{ color: '#6B7280' }}>
                  {user.permissions?.length ?? 0} permiso(s) especial(es)
                </p>
                <p className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
                  Los permisos especiales se aplican además de los permisos heredados por roles.
                </p>
              </div>
              <Button size="sm" variant="outline" onClick={() => setShowPermisoModal(true)} className="shrink-0 gap-1.5 text-xs">
                <Plus className="h-3.5 w-3.5" />
                Asignar permiso
              </Button>
            </div>
            <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
              {user.permissions && user.permissions.length > 0 ? (
                user.permissions.map((perm) => {
                  const isTemporal = !!perm.valid_until
                  return (
                    <div key={perm.id} className="px-5 py-3 flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-sm font-medium" style={{ color: '#1A1A2E' }}>
                            {perm.code}
                          </span>
                          {isTemporal && <Badge variant="temporal" className="text-xs">Temporal</Badge>}
                        </div>
                        <p className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
                          {perm.valid_until
                            ? `Expira ${formatDateRelative(perm.valid_until)}`
                            : 'Sin expiración'}
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setConfirmRevoke({ type: 'permission', id: perm.id, label: perm.code })}
                        aria-label={`Revocar permiso ${perm.code}`}
                      >
                        <Trash2 className="h-4 w-4 text-red-500" />
                      </Button>
                    </div>
                  )
                })
              ) : (
                <div className="py-10 text-center text-sm" style={{ color: '#9CA3AF' }}>
                  Sin permisos especiales asignados.
                </div>
              )}
            </div>
          </div>
        )}

        {/* ── Cost Centers tab ──────────────────────────── */}
        {activeTab === 'cost_centers' && (
          <div>
            <div
              className="flex items-center justify-between px-5 py-3"
              style={{ borderBottom: '1px solid #F3F4F6' }}
            >
              <p className="text-sm font-medium" style={{ color: '#6B7280' }}>
                {user.cost_centers?.length ?? 0} centro(s) de costo asignado(s)
              </p>
              <Button size="sm" variant="outline" onClick={() => setShowCeCosModal(true)} className="gap-1.5 text-xs">
                <Plus className="h-3.5 w-3.5" />
                Asignar CeCos
              </Button>
            </div>
            <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
              {user.cost_centers && user.cost_centers.length > 0 ? (
                user.cost_centers.map((cc) => (
                  <div key={cc.id} className="px-5 py-3 flex items-center gap-4">
                    <span className="font-mono text-xs font-semibold px-2 py-1 rounded" style={{ backgroundColor: '#F3F4F6', color: '#374151' }}>
                      {cc.code}
                    </span>
                    <span className="text-sm" style={{ color: '#1A1A2E' }}>{cc.name}</span>
                  </div>
                ))
              ) : (
                <div className="py-10 text-center text-sm" style={{ color: '#9CA3AF' }}>
                  Sin centros de costo asignados.
                </div>
              )}
            </div>
          </div>
        )}

        {/* ── Audit tab ─────────────────────────────────── */}
        {activeTab === 'audit' && <AuditTab userId={user.id} />}
      </div>

      {/* Modals */}
      <AsignarRolModal
        open={showRolModal}
        onOpenChange={setShowRolModal}
        userId={user.id}
        username={user.username}
        onSuccess={loadUser}
      />
      <AsignarPermisoModal
        open={showPermisoModal}
        onOpenChange={setShowPermisoModal}
        userId={user.id}
        username={user.username}
        onSuccess={loadUser}
      />
      <AsignarCeCosModal
        open={showCeCosModal}
        onOpenChange={setShowCeCosModal}
        userId={user.id}
        username={user.username}
        onSuccess={loadUser}
      />

      {confirmRevoke && (
        <ConfirmDialog
          open
          onOpenChange={(open) => { if (!open) setConfirmRevoke(null) }}
          title={`Revocar ${confirmRevoke.type === 'role' ? 'rol' : 'permiso'}`}
          description={`¿Revocar "${confirmRevoke.label}" del usuario @${user.username}?`}
          confirmLabel="Revocar"
          variant="destructive"
          onConfirm={handleRevoke}
          isLoading={isRevoking}
        />
      )}
    </div>
  )
}

// ─── Stat helper ────────────────────────────────────────────────────────────

function Stat({ label, value, valueColor }: { label: string; value: string; valueColor?: string }) {
  return (
    <div>
      <p className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>
        {label}
      </p>
      <p className="text-sm font-medium" style={{ color: valueColor ?? '#374151' }}>
        {value}
      </p>
    </div>
  )
}

import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Users,
  Shield,
  Key,
  Monitor,
  ArrowRight,
  AlertTriangle,
  Lock,
  Building2,
  ClipboardList,
} from 'lucide-react'
import { usersApi } from '@/api/users'
import { rolesApi } from '@/api/roles'
import { permissionsApi } from '@/api/permissions'
import { auditApi } from '@/api/audit'
import { applicationsApi } from '@/api/applications'
import type { AuditLog, User } from '@/types'
import { formatDateRelative } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'

// ─── Event badge mapping ────────────────────────────────────────────────────

const auditEventVariants: Record<string, 'success' | 'warning' | 'secondary' | 'destructive'> = {
  AUTH_LOGIN_SUCCESS: 'success',
  AUTH_LOGOUT: 'secondary',
  AUTH_LOGIN_FAILED: 'warning',
  AUTH_ACCOUNT_LOCKED: 'destructive',
  USER_CREATED: 'success',
  USER_UPDATED: 'secondary',
  USER_DEACTIVATED: 'warning',
  USER_UNLOCKED: 'success',
  ROLE_CREATED: 'success',
  ROLE_DELETED: 'destructive',
}

function getEventVariant(eventType: string) {
  return auditEventVariants[eventType] ?? 'secondary'
}

// Readable short label for event types
function eventLabel(type: string): string {
  const map: Record<string, string> = {
    AUTH_LOGIN_SUCCESS: 'Login',
    AUTH_LOGIN_FAILED: 'Login fallido',
    AUTH_LOGOUT: 'Logout',
    AUTH_ACCOUNT_LOCKED: 'Cuenta bloqueada',
    AUTH_TOKEN_REFRESHED: 'Token renovado',
    AUTH_PASSWORD_CHANGED: 'Contraseña cambiada',
    AUTH_PASSWORD_RESET: 'Password reset',
    USER_CREATED: 'Usuario creado',
    USER_UPDATED: 'Usuario actualizado',
    USER_DEACTIVATED: 'Usuario desactivado',
    USER_UNLOCKED: 'Usuario desbloqueado',
    USER_ROLE_ASSIGNED: 'Rol asignado',
    USER_ROLE_REVOKED: 'Rol revocado',
    USER_PERMISSION_ASSIGNED: 'Permiso asignado',
    USER_PERMISSION_REVOKED: 'Permiso revocado',
    ROLE_CREATED: 'Rol creado',
    ROLE_DELETED: 'Rol eliminado',
    ROLE_UPDATED: 'Rol actualizado',
    PERMISSION_CREATED: 'Permiso creado',
    PERMISSION_DELETED: 'Permiso eliminado',
  }
  return map[type] ?? type
}

function actorLabel(actorId: string | null): string {
  if (!actorId) return 'Sistema'
  return actorId.slice(0, 8)
}

// ─── Stat card ──────────────────────────────────────────────────────────────

interface StatCardProps {
  label: string
  value: number | string
  sub: string
  icon: React.ComponentType<{ className?: string }>
  iconBg: string
  iconColor: string
  href: string
  isLoading: boolean
}

function StatCard({ label, value, sub, icon: Icon, iconBg, iconColor, href, isLoading }: StatCardProps) {
  return (
    <Link
      to={href}
      className="bg-white rounded-xl p-5 transition-all group"
      style={{ border: '1px solid #E0E5EC', boxShadow: '0 1px 3px rgba(0,0,0,0.04)' }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLElement
        el.style.borderColor = '#004899'
        el.style.boxShadow = '0 2px 8px rgba(0,72,153,0.12)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLElement
        el.style.borderColor = '#E0E5EC'
        el.style.boxShadow = '0 1px 3px rgba(0,0,0,0.04)'
      }}
    >
      <div className="flex items-start justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-wide" style={{ color: '#6B7280' }}>
            {label}
          </p>
          <p className="mt-1 text-2xl font-bold" style={{ color: '#1A1A2E' }}>
            {isLoading ? (
              <span className="inline-block h-7 w-16 animate-pulse rounded" style={{ backgroundColor: '#E0E5EC' }} />
            ) : (
              value
            )}
          </p>
          <p className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
            {sub}
          </p>
        </div>
        <div className="rounded-lg p-2.5" style={{ backgroundColor: iconBg }}>
          <Icon className={`h-5 w-5 ${iconColor}`} aria-hidden="true" />
        </div>
      </div>
    </Link>
  )
}

// ─── Dashboard ──────────────────────────────────────────────────────────────

interface Stats {
  totalUsers: number
  activeUsers: number
  totalApplications: number
  activeApplications: number
  totalRoles: number
  totalPermissions: number
}

const quickLinks = [
  { to: '/users', label: 'Gestionar usuarios', icon: Users },
  { to: '/applications', label: 'Gestionar aplicaciones', icon: Monitor },
  { to: '/roles', label: 'Gestionar roles', icon: Shield },
  { to: '/cost-centers', label: 'Centros de costo', icon: Building2 },
  { to: '/audit', label: 'Ver auditoría', icon: ClipboardList },
]

export function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [recentLogs, setRecentLogs] = useState<AuditLog[]>([])
  const [lockedUsers, setLockedUsers] = useState<User[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const load = async () => {
      setIsLoading(true)
      try {
        const [usersAll, usersActive, apps, roles, permissions, logs, usersPage] =
          await Promise.allSettled([
            usersApi.list({ page: 1, page_size: 1 }),
            usersApi.list({ page: 1, page_size: 1, is_active: true }),
            applicationsApi.list({ page: 1, page_size: 1 }),
            rolesApi.list({ page: 1, page_size: 1 }),
            permissionsApi.list({ page: 1, page_size: 1 }),
            auditApi.list({ page: 1, page_size: 10 }),
            // Fetch first 100 users to detect locked accounts
            usersApi.list({ page: 1, page_size: 100 }),
          ])

        setStats({
          totalUsers: usersAll.status === 'fulfilled' ? usersAll.value.total : 0,
          activeUsers: usersActive.status === 'fulfilled' ? usersActive.value.total : 0,
          totalApplications: apps.status === 'fulfilled' ? apps.value.total : 0,
          activeApplications:
            apps.status === 'fulfilled'
              ? apps.value.data.filter((a) => a.is_active).length
              : 0,
        totalRoles: roles.status === 'fulfilled' ? roles.value.total : 0,
          totalPermissions: permissions.status === 'fulfilled' ? permissions.value.total : 0,
        })

        if (logs.status === 'fulfilled') {
          setRecentLogs(logs.value.data)
        }

        if (usersPage.status === 'fulfilled') {
          setLockedUsers(usersPage.value.data.filter((u) => u.locked_until !== null))
        }
      } finally {
        setIsLoading(false)
      }
    }
    void load()
  }, [])

  const statCards: StatCardProps[] = [
    {
      label: 'Usuarios',
      value: stats?.totalUsers ?? 0,
      sub: `${stats?.activeUsers ?? 0} activos`,
      icon: Users,
      iconBg: '#EFF6FF',
      iconColor: 'text-sodexo-blue',
      href: '/users',
      isLoading,
    },
    {
      label: 'Aplicaciones',
      value: stats?.totalApplications ?? 0,
      sub: `${stats?.activeApplications ?? 0} activas`,
      icon: Monitor,
      iconBg: '#F0FDF4',
      iconColor: 'text-green-600',
      href: '/applications',
      isLoading,
    },
    {
      label: 'Roles',
      value: stats?.totalRoles ?? 0,
      sub: 'Roles de acceso',
      icon: Shield,
      iconBg: '#FAF5FF',
      iconColor: 'text-purple-600',
      href: '/roles',
      isLoading,
    },
    {
      label: 'Permisos',
      value: stats?.totalPermissions ?? 0,
      sub: 'Permisos del sistema',
      icon: Key,
      iconBg: '#FFF7ED',
      iconColor: 'text-orange-600',
      href: '/permissions',
      isLoading,
    },
  ]

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold" style={{ color: '#1A1A2E' }}>
          Dashboard
        </h1>
        <p className="text-sm mt-1" style={{ color: '#6B7280' }}>
          Resumen del sistema Sentinel
        </p>
      </div>

      {/* Metric cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {statCards.map((card) => (
          <StatCard key={card.label} {...card} />
        ))}
      </div>

      {/* Bottom grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Recent audit logs — 2/3 width */}
        <div
          className="lg:col-span-2 bg-white rounded-xl overflow-hidden"
          style={{ border: '1px solid #E0E5EC' }}
        >
          <div
            className="flex items-center justify-between px-5 py-4"
            style={{ borderBottom: '1px solid #F3F4F6' }}
          >
            <h2 className="font-semibold" style={{ color: '#1A1A2E' }}>
              Actividad reciente
            </h2>
            <Link
              to="/audit"
              className="text-sm flex items-center gap-1 transition-colors"
              style={{ color: '#004899' }}
              onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.color = '#0066CC')}
              onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.color = '#004899')}
            >
              Ver todos los logs <ArrowRight className="h-3 w-3" />
            </Link>
          </div>

          {/* Table header */}
          <div
            className="grid grid-cols-[2fr_1fr_1fr] gap-2 px-5 py-2 text-xs font-medium uppercase tracking-wide"
            style={{ backgroundColor: '#F9FAFB', color: '#9CA3AF', borderBottom: '1px solid #F3F4F6' }}
          >
            <span>Evento</span>
            <span>Actor</span>
            <span className="text-right">Hace</span>
          </div>

          <div className="divide-y" style={{ borderColor: '#F9FAFB' }}>
            {isLoading
              ? Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="px-5 py-3 grid grid-cols-[2fr_1fr_1fr] gap-2 items-center">
                    <div className="h-5 w-32 animate-pulse rounded" style={{ backgroundColor: '#F3F4F6' }} />
                    <div className="h-4 w-16 animate-pulse rounded" style={{ backgroundColor: '#F3F4F6' }} />
                    <div className="h-4 w-12 animate-pulse rounded ml-auto" style={{ backgroundColor: '#F3F4F6' }} />
                  </div>
                ))
              : recentLogs.length === 0
              ? (
                <div className="px-5 py-10 text-center text-sm" style={{ color: '#9CA3AF' }}>
                  No hay eventos de auditoría aún.
                </div>
              )
              : recentLogs.map((log) => (
                  <div
                    key={log.id}
                    className="px-5 py-3 grid grid-cols-[2fr_1fr_1fr] gap-2 items-center hover:bg-gray-50/50 transition-colors"
                  >
                    <Badge variant={getEventVariant(log.event_type)} className="text-xs w-fit">
                      {eventLabel(log.event_type)}
                    </Badge>
                    <span className="text-xs font-mono truncate" style={{ color: '#6B7280' }}>
                      {actorLabel(log.actor_id)}
                    </span>
                    <span className="text-xs text-right shrink-0" style={{ color: '#9CA3AF' }}>
                      {formatDateRelative(log.created_at)}
                    </span>
                  </div>
                ))}
          </div>
        </div>

        {/* Right column: Alerts + Quick links — 1/3 width */}
        <div className="space-y-4">
          {/* Alerts panel */}
          <div
            className="bg-white rounded-xl overflow-hidden"
            style={{ border: '1px solid #E0E5EC' }}
          >
            <div
              className="px-5 py-4"
              style={{ borderBottom: '1px solid #F3F4F6' }}
            >
              <h2 className="font-semibold" style={{ color: '#1A1A2E' }}>
                Alertas del sistema
              </h2>
            </div>
            <div className="p-4 space-y-3">
              {/* Locked accounts */}
              {isLoading ? (
                <div className="h-14 animate-pulse rounded-lg" style={{ backgroundColor: '#F3F4F6' }} />
              ) : lockedUsers.length > 0 ? (
                <Link
                  to="/users"
                  className="flex items-start gap-3 rounded-lg p-3 transition-colors"
                  style={{ backgroundColor: '#FEF3C7', border: '1px solid #FDE68A' }}
                  onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#FDE68A')}
                  onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#FEF3C7')}
                >
                  <Lock className="h-4 w-4 mt-0.5 shrink-0 text-amber-600" aria-hidden="true" />
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-amber-900">
                      {lockedUsers.length} cuenta{lockedUsers.length > 1 ? 's' : ''} bloqueada{lockedUsers.length > 1 ? 's' : ''}
                    </p>
                    <p className="text-xs text-amber-700 mt-0.5">Ver usuarios →</p>
                  </div>
                </Link>
              ) : (
                <div
                  className="flex items-center gap-3 rounded-lg p-3"
                  style={{ backgroundColor: '#F0FDF4', border: '1px solid #BBF7D0' }}
                >
                  <Shield className="h-4 w-4 shrink-0 text-green-600" aria-hidden="true" />
                  <p className="text-sm text-green-800">Sin cuentas bloqueadas</p>
                </div>
              )}

              {/* Must change password */}
              {!isLoading && (() => {
                // This would require fetching users with must_change_pwd — omit for now
                return null
              })()}

              {/* Info: no pending alerts */}
              {!isLoading && lockedUsers.length === 0 && (
                <p className="text-xs px-1" style={{ color: '#9CA3AF' }}>
                  No hay alertas pendientes.
                </p>
              )}
            </div>
          </div>

          {/* Quick links */}
          <div
            className="bg-white rounded-xl overflow-hidden"
            style={{ border: '1px solid #E0E5EC' }}
          >
            <div className="px-5 py-4" style={{ borderBottom: '1px solid #F3F4F6' }}>
              <h2 className="font-semibold" style={{ color: '#1A1A2E' }}>
                Accesos rápidos
              </h2>
            </div>
            <div className="p-3 space-y-0.5">
              {quickLinks.map(({ to, label, icon: Icon }) => (
                <Link
                  key={to}
                  to={to}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-md text-sm transition-colors group"
                  style={{ color: '#374151' }}
                  onMouseEnter={(e) => {
                    const el = e.currentTarget as HTMLElement
                    el.style.backgroundColor = '#EFF6FF'
                    el.style.color = '#004899'
                  }}
                  onMouseLeave={(e) => {
                    const el = e.currentTarget as HTMLElement
                    el.style.backgroundColor = ''
                    el.style.color = '#374151'
                  }}
                >
                  <Icon className="h-4 w-4 shrink-0" style={{ color: '#9CA3AF' }} aria-hidden="true" />
                  <span className="truncate">{label}</span>
                  <ArrowRight className="h-3 w-3 ml-auto shrink-0" style={{ color: '#D1D5DB' }} />
                </Link>
              ))}
            </div>
          </div>

          {/* Alert triangle info */}
          <div
            className="flex items-start gap-2 rounded-lg px-4 py-3"
            style={{ backgroundColor: '#EFF6FF', border: '1px solid #BFDBFE' }}
          >
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" style={{ color: '#004899' }} aria-hidden="true" />
            <p className="text-xs" style={{ color: '#1D4ED8' }}>
              Los datos de cuentas bloqueadas corresponden a los primeros 100 usuarios cargados.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

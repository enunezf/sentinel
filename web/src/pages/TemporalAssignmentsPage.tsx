import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Clock, AlertTriangle, ExternalLink, RefreshCw } from 'lucide-react'
import { usersApi } from '@/api/users'
import type { User, UserRole, UserPermission } from '@/types'
import { PageHeader } from '@/components/shared/PageHeader'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/hooks/useToast'
import { formatDate, formatDateRelative } from '@/lib/utils'

// ─── Types ───────────────────────────────────────────────────────────────────

interface TemporalRole {
  type: 'role'
  user: Pick<User, 'id' | 'username' | 'email'>
  assignment: UserRole
  expiresAt: Date
  hoursLeft: number
}

interface TemporalPermission {
  type: 'permission'
  user: Pick<User, 'id' | 'username' | 'email'>
  assignment: UserPermission
  expiresAt: Date
  hoursLeft: number
}

type TemporalItem = TemporalRole | TemporalPermission

// ─── Helpers ─────────────────────────────────────────────────────────────────

const THRESHOLD_HOURS = 48

function urgencyColor(hours: number) {
  if (hours < 0) return { bg: '#FEE2E2', text: '#D0021B', border: '#FECACA' }
  if (hours < 6) return { bg: '#FEE2E2', text: '#D0021B', border: '#FECACA' }
  if (hours < 24) return { bg: '#FEF3C7', text: '#D97706', border: '#FDE68A' }
  return { bg: '#FFF7ED', text: '#EA580C', border: '#FED7AA' }
}

function UrgencyBadge({ hours }: { hours: number }) {
  const c = urgencyColor(hours)
  const label =
    hours < 0 ? 'Expirado' : hours < 1 ? '< 1 hora' : hours < 24 ? `${Math.round(hours)}h` : `${Math.round(hours / 24)}d`
  return (
    <span
      className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full font-semibold"
      style={{ backgroundColor: c.bg, color: c.text, border: `1px solid ${c.border}` }}
    >
      <Clock className="h-3 w-3 shrink-0" />
      {label}
    </span>
  )
}

// ─── Page ────────────────────────────────────────────────────────────────────

export function TemporalAssignmentsPage() {
  const navigate = useNavigate()
  const [items, setItems] = useState<TemporalItem[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [filter, setFilter] = useState<'all' | 'expired' | 'critical' | 'warning'>('all')

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      // Load first page of users
      const userList = await usersApi.list({ page: 1, page_size: 50 })

      // Fetch full details for each user in parallel
      const details = await Promise.allSettled(
        userList.data.map((u) => usersApi.get(u.id))
      )

      const now = new Date()
      const collected: TemporalItem[] = []

      details.forEach((result, i) => {
        if (result.status !== 'fulfilled') return
        const user = result.value
        const userRef = { id: user.id, username: user.username, email: user.email }

        // Roles with valid_until
        ;(user.roles ?? []).forEach((role) => {
          if (!role.valid_until) return
          const expiresAt = new Date(role.valid_until)
          const hoursLeft = (expiresAt.getTime() - now.getTime()) / 3_600_000
          if (hoursLeft > THRESHOLD_HOURS) return
          collected.push({ type: 'role', user: userRef, assignment: role, expiresAt, hoursLeft })
        })

        // Permissions with valid_until
        ;(user.permissions ?? []).forEach((perm) => {
          if (!perm.valid_until) return
          const expiresAt = new Date(perm.valid_until)
          const hoursLeft = (expiresAt.getTime() - now.getTime()) / 3_600_000
          if (hoursLeft > THRESHOLD_HOURS) return
          collected.push({ type: 'permission', user: userRef, assignment: perm, expiresAt, hoursLeft })
        })

        // Suppress unused index warning
        void i
      })

      // Sort by closest expiration (expired first, then soonest)
      collected.sort((a, b) => a.expiresAt.getTime() - b.expiresAt.getTime())
      setItems(collected)
    } catch {
      toast({ title: 'Error al cargar asignaciones temporales', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const filtered = items.filter((item) => {
    if (filter === 'expired') return item.hoursLeft < 0
    if (filter === 'critical') return item.hoursLeft >= 0 && item.hoursLeft < 6
    if (filter === 'warning') return item.hoursLeft >= 6 && item.hoursLeft <= THRESHOLD_HOURS
    return true
  })

  const expiredCount = items.filter((i) => i.hoursLeft < 0).length
  const criticalCount = items.filter((i) => i.hoursLeft >= 0 && i.hoursLeft < 6).length
  const warningCount = items.filter((i) => i.hoursLeft >= 6).length

  const FILTERS = [
    { value: 'all' as const, label: `Todas (${items.length})` },
    { value: 'expired' as const, label: `Expiradas (${expiredCount})` },
    { value: 'critical' as const, label: `Críticas < 6h (${criticalCount})` },
    { value: 'warning' as const, label: `Advertencia (${warningCount})` },
  ]

  return (
    <div>
      <PageHeader
        title="Asignaciones Temporales"
        description="Roles y permisos con fecha de vencimiento próxima o expirados"
        actions={
          <Button variant="outline" onClick={() => { void load() }} disabled={isLoading}>
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
            Actualizar
          </Button>
        }
      />

      {/* Summary cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-5">
        {[
          { label: 'Expiradas', count: expiredCount, color: '#D0021B', bg: '#FEE2E2', border: '#FECACA' },
          { label: 'Críticas (< 6h)', count: criticalCount, color: '#D97706', bg: '#FEF3C7', border: '#FDE68A' },
          { label: 'Advertencia (< 48h)', count: warningCount, color: '#EA580C', bg: '#FFF7ED', border: '#FED7AA' },
        ].map(({ label, count, color, bg, border }) => (
          <div
            key={label}
            className="rounded-xl p-4 flex items-center gap-3"
            style={{ backgroundColor: bg, border: `1px solid ${border}` }}
          >
            <AlertTriangle className="h-5 w-5 shrink-0" style={{ color }} />
            <div>
              <p className="text-2xl font-bold" style={{ color }}>{count}</p>
              <p className="text-xs font-medium" style={{ color }}>{label}</p>
            </div>
          </div>
        ))}
      </div>

      {/* Filter pills */}
      <div className="flex items-center gap-2 mb-4 flex-wrap">
        {FILTERS.map(({ value, label }) => (
          <button
            key={value}
            onClick={() => setFilter(value)}
            className="px-3 py-1 rounded-full text-xs font-medium transition-colors"
            style={
              filter === value
                ? { backgroundColor: '#004899', color: '#FFFFFF' }
                : { backgroundColor: '#F3F4F6', color: '#374151' }
            }
          >
            {label}
          </button>
        ))}
      </div>

      {/* Table */}
      <div
        className="rounded-xl overflow-hidden"
        style={{ border: '1px solid #E0E5EC' }}
      >
        <table className="w-full text-sm">
          <thead style={{ backgroundColor: '#F4F6F9', borderBottom: '1px solid #E0E5EC' }}>
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                Usuario
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                Tipo
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                Asignación
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                Aplicación
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                Vence
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                Estado
              </th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody style={{ backgroundColor: '#FFFFFF' }}>
            {isLoading ? (
              <tr>
                <td colSpan={7} className="px-4 py-16 text-center" style={{ color: '#9CA3AF' }}>
                  <div className="flex items-center justify-center gap-2">
                    <span
                      className="h-4 w-4 animate-spin rounded-full border-2 border-t-transparent"
                      style={{ borderColor: '#004899', borderTopColor: 'transparent' }}
                    />
                    Cargando asignaciones...
                  </div>
                </td>
              </tr>
            ) : filtered.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-16 text-center" style={{ color: '#9CA3AF' }}>
                  {items.length === 0
                    ? 'No hay asignaciones temporales próximas a vencer.'
                    : 'No hay asignaciones que coincidan con el filtro seleccionado.'}
                </td>
              </tr>
            ) : (
              filtered.map((item, idx) => {
                const isRole = item.type === 'role'
                const name = isRole
                  ? (item as TemporalRole).assignment.name
                  : (item as TemporalPermission).assignment.code
                const app = isRole
                  ? (item as TemporalRole).assignment.application
                  : (item as TemporalPermission).assignment.application

                return (
                  <tr
                    key={idx}
                    style={{ borderTop: idx > 0 ? '1px solid #F3F4F6' : undefined }}
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <div
                          className="h-7 w-7 rounded-full flex items-center justify-center text-xs font-bold text-white shrink-0"
                          style={{ backgroundColor: '#004899' }}
                        >
                          {item.user.username.slice(0, 2).toUpperCase()}
                        </div>
                        <div>
                          <p className="text-xs font-medium" style={{ color: '#1A1A2E' }}>
                            {item.user.username}
                          </p>
                          <p className="text-xs" style={{ color: '#9CA3AF' }}>{item.user.email}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={isRole ? 'secondary' : 'default'}
                        className="text-xs"
                      >
                        {isRole ? 'Rol' : 'Permiso'}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className="text-xs font-mono px-1.5 py-0.5 rounded"
                        style={{ backgroundColor: '#F3F4F6', color: '#374151' }}
                      >
                        {name}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-xs" style={{ color: '#6B7280' }}>{app}</span>
                    </td>
                    <td className="px-4 py-3">
                      <div>
                        <p className="text-xs" style={{ color: '#374151' }}>
                          {formatDate(item.expiresAt.toISOString())}
                        </p>
                        <p className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
                          {formatDateRelative(item.expiresAt.toISOString())}
                        </p>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <UrgencyBadge hours={item.hoursLeft} />
                    </td>
                    <td className="px-4 py-3">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => void navigate(`/users/${item.user.id}`)}
                        className="text-xs"
                      >
                        <ExternalLink className="h-3.5 w-3.5 mr-1" />
                        Ver usuario
                      </Button>
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>

      {!isLoading && items.length > 0 && (
        <p className="text-xs mt-3" style={{ color: '#9CA3AF' }}>
          Mostrando asignaciones con vencimiento en las próximas {THRESHOLD_HOURS}h (primeros 50 usuarios).
        </p>
      )}
    </div>
  )
}

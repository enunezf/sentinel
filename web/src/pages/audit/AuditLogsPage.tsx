import { useEffect, useState, useCallback } from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { Filter, Download, X } from 'lucide-react'
import { auditApi } from '@/api/audit'
import type { ListAuditLogsParams } from '@/api/audit'
import type { AuditLog } from '@/types'
import { PageHeader } from '@/components/shared/PageHeader'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from '@/hooks/useToast'
import { usePagination } from '@/hooks/usePagination'
import { formatDate, formatDateRelative } from '@/lib/utils'

// ─── Event catalogue ────────────────────────────────────────────────────────

const EVENT_CATEGORIES: Record<string, string[]> = {
  AUTH: [
    'AUTH_LOGIN_SUCCESS',
    'AUTH_LOGIN_FAILED',
    'AUTH_ACCOUNT_LOCKED',
    'AUTH_LOGOUT',
    'AUTH_TOKEN_REFRESHED',
    'AUTH_PASSWORD_CHANGED',
    'AUTH_PASSWORD_RESET',
  ],
  USER: [
    'USER_CREATED',
    'USER_UPDATED',
    'USER_DEACTIVATED',
    'USER_UNLOCKED',
    'USER_ROLE_ASSIGNED',
    'USER_ROLE_REVOKED',
    'USER_PERMISSION_ASSIGNED',
    'USER_PERMISSION_REVOKED',
    'USER_COST_CENTER_ASSIGNED',
  ],
  ROLE: [
    'ROLE_CREATED',
    'ROLE_UPDATED',
    'ROLE_DELETED',
    'ROLE_PERMISSION_ASSIGNED',
    'ROLE_PERMISSION_REVOKED',
  ],
  PERMISSION: ['PERMISSION_CREATED', 'PERMISSION_DELETED'],
}

const EVENT_LABEL: Record<string, string> = {
  AUTH_LOGIN_SUCCESS: 'Login exitoso',
  AUTH_LOGIN_FAILED: 'Login fallido',
  AUTH_ACCOUNT_LOCKED: 'Cuenta bloqueada',
  AUTH_LOGOUT: 'Logout',
  AUTH_TOKEN_REFRESHED: 'Token renovado',
  AUTH_PASSWORD_CHANGED: 'Contraseña cambiada',
  AUTH_PASSWORD_RESET: 'Contraseña restablecida',
  USER_CREATED: 'Usuario creado',
  USER_UPDATED: 'Usuario actualizado',
  USER_DEACTIVATED: 'Usuario desactivado',
  USER_UNLOCKED: 'Usuario desbloqueado',
  USER_ROLE_ASSIGNED: 'Rol asignado',
  USER_ROLE_REVOKED: 'Rol revocado',
  USER_PERMISSION_ASSIGNED: 'Permiso asignado',
  USER_PERMISSION_REVOKED: 'Permiso revocado',
  USER_COST_CENTER_ASSIGNED: 'CeCo asignado',
  ROLE_CREATED: 'Rol creado',
  ROLE_UPDATED: 'Rol actualizado',
  ROLE_DELETED: 'Rol eliminado',
  ROLE_PERMISSION_ASSIGNED: 'Permiso asig. a rol',
  ROLE_PERMISSION_REVOKED: 'Permiso rev. de rol',
  PERMISSION_CREATED: 'Permiso creado',
  PERMISSION_DELETED: 'Permiso eliminado',
}

function eventLabel(code: string): string {
  return EVENT_LABEL[code] ?? code
}

function eventBadgeVariant(
  eventType: string,
): 'default' | 'success' | 'warning' | 'destructive' | 'secondary' {
  if (
    eventType.includes('FAILED') ||
    eventType.includes('LOCKED') ||
    eventType.includes('DELETED') ||
    eventType.includes('REVOKED')
  )
    return 'destructive'
  if (
    eventType.includes('SUCCESS') ||
    eventType.includes('CREATED') ||
    eventType.includes('ASSIGNED') ||
    eventType.includes('UNLOCKED')
  )
    return 'success'
  if (
    eventType.includes('UPDATED') ||
    eventType.includes('DEACTIVATED') ||
    eventType.includes('RESET') ||
    eventType.includes('CHANGED')
  )
    return 'warning'
  return 'secondary'
}

// ─── Detail Modal ────────────────────────────────────────────────────────────

function DetailModal({ log, onClose }: { log: AuditLog; onClose: () => void }) {
  const hasValues = log.old_value !== null || log.new_value !== null

  return (
    <DialogPrimitive.Root open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm"
        />
        <DialogPrimitive.Content
          className="fixed left-1/2 top-1/2 z-50 w-full max-w-2xl max-h-[90vh] overflow-y-auto -translate-x-1/2 -translate-y-1/2 rounded-xl bg-white shadow-2xl focus:outline-none"
        >
          {/* Header */}
          <div
            className="flex items-center justify-between px-6 py-4 border-b"
            style={{ borderColor: '#E0E5EC' }}
          >
            <div className="flex items-center gap-3">
              <Badge variant={eventBadgeVariant(log.event_type)} className="text-xs">
                {eventLabel(log.event_type)}
              </Badge>
              <span className="text-xs" style={{ color: '#9CA3AF' }}>
                {formatDate(log.created_at)}
              </span>
            </div>
            <button
              onClick={onClose}
              className="rounded-full p-1 transition-colors"
              style={{ color: '#9CA3AF' }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = '#F3F4F6'; e.currentTarget.style.color = '#374151' }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent'; e.currentTarget.style.color = '#9CA3AF' }}
              aria-label="Cerrar"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {/* Body */}
          <div className="px-6 py-4 space-y-4">
            {/* Meta */}
            <div
              className="grid grid-cols-2 gap-3 text-xs rounded-lg p-4"
              style={{ backgroundColor: '#F4F6F9' }}
            >
              <div>
                <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>ID de evento</p>
                <p className="font-mono" style={{ color: '#1A1A2E' }}>{log.id}</p>
              </div>
              <div>
                <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>Resultado</p>
                <Badge variant={log.success ? 'success' : 'destructive'} className="text-xs">
                  {log.success ? 'Éxito' : 'Fallo'}
                </Badge>
              </div>
              {log.user_id && (
                <div>
                  <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>Usuario (ID)</p>
                  <p className="font-mono" style={{ color: '#1A1A2E' }}>{log.user_id}</p>
                </div>
              )}
              {log.actor_id && (
                <div>
                  <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>Actor (ID)</p>
                  <p className="font-mono" style={{ color: '#1A1A2E' }}>{log.actor_id}</p>
                </div>
              )}
              {log.resource_type && (
                <div>
                  <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>Recurso</p>
                  <p className="font-mono" style={{ color: '#1A1A2E' }}>
                    {log.resource_type}
                    {log.resource_id ? ` / ${log.resource_id}` : ''}
                  </p>
                </div>
              )}
              {log.ip_address && (
                <div>
                  <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>Dirección IP</p>
                  <p className="font-mono" style={{ color: '#1A1A2E' }}>{log.ip_address}</p>
                </div>
              )}
              {log.user_agent && (
                <div className="col-span-2">
                  <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>User-Agent</p>
                  <p
                    className="font-mono truncate"
                    style={{ color: '#374151' }}
                    title={log.user_agent}
                  >
                    {log.user_agent}
                  </p>
                </div>
              )}
              <div>
                <p className="font-medium mb-0.5" style={{ color: '#6B7280' }}>Hace</p>
                <p style={{ color: '#374151' }}>{formatDateRelative(log.created_at)}</p>
              </div>
            </div>

            {/* Values diff */}
            {hasValues && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {log.old_value !== null && (
                  <div>
                    <p
                      className="text-xs font-semibold mb-1.5 flex items-center gap-1.5"
                      style={{ color: '#6B7280' }}
                    >
                      <span
                        className="inline-block w-2 h-2 rounded-full"
                        style={{ backgroundColor: '#D0021B' }}
                      />
                      Valor anterior
                    </p>
                    <pre
                      className="text-xs font-mono rounded-lg p-3 overflow-auto max-h-48"
                      style={{
                        backgroundColor: '#FFF5F5',
                        border: '1px solid #FEE2E2',
                        color: '#7F1D1D',
                      }}
                    >
                      {JSON.stringify(log.old_value, null, 2)}
                    </pre>
                  </div>
                )}
                {log.new_value !== null && (
                  <div>
                    <p
                      className="text-xs font-semibold mb-1.5 flex items-center gap-1.5"
                      style={{ color: '#6B7280' }}
                    >
                      <span
                        className="inline-block w-2 h-2 rounded-full"
                        style={{ backgroundColor: '#16A34A' }}
                      />
                      Nuevo valor
                    </p>
                    <pre
                      className="text-xs font-mono rounded-lg p-3 overflow-auto max-h-48"
                      style={{
                        backgroundColor: '#F0FDF4',
                        border: '1px solid #BBF7D0',
                        color: '#14532D',
                      }}
                    >
                      {JSON.stringify(log.new_value, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            )}

            {/* Error */}
            {log.error_message && (
              <div>
                <p
                  className="text-xs font-semibold mb-1.5"
                  style={{ color: '#D0021B' }}
                >
                  Mensaje de error
                </p>
                <p
                  className="text-xs rounded-lg p-3"
                  style={{ backgroundColor: '#FEF2F2', border: '1px solid #FECACA', color: '#7F1D1D' }}
                >
                  {log.error_message}
                </p>
              </div>
            )}

            {!hasValues && !log.error_message && (
              <p className="text-xs text-center py-4" style={{ color: '#9CA3AF' }}>
                Sin datos adicionales para este evento.
              </p>
            )}
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

// ─── CSV Export ──────────────────────────────────────────────────────────────

function exportToCsv(items: AuditLog[]) {
  const headers = ['ID', 'Timestamp', 'Evento', 'Usuario ID', 'Actor ID', 'Recurso', 'IP', 'Resultado', 'Error']
  const rows = items.map((log) => [
    log.id,
    log.created_at,
    log.event_type,
    log.user_id ?? '',
    log.actor_id ?? '',
    log.resource_type ? `${log.resource_type}/${log.resource_id ?? ''}` : '',
    log.ip_address ?? '',
    log.success ? 'Éxito' : 'Fallo',
    log.error_message ?? '',
  ])

  const csvContent = [headers, ...rows]
    .map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(','))
    .join('\n')

  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `auditoria_${new Date().toISOString().slice(0, 10)}.csv`
  link.click()
  URL.revokeObjectURL(url)
}

// ─── Page ────────────────────────────────────────────────────────────────────

export function AuditLogsPage() {
  const { page, pageSize, goToPage, reset } = usePagination()
  const [data, setData] = useState<{ items: AuditLog[]; total: number; totalPages: number }>({
    items: [],
    total: 0,
    totalPages: 0,
  })
  const [isLoading, setIsLoading] = useState(false)
  const [selectedLog, setSelectedLog] = useState<AuditLog | null>(null)

  // Filters
  const [filters, setFilters] = useState({
    userId: '',
    actorId: '',
    category: '',
    eventType: '',
    fromDate: '',
    toDate: '',
    success: '',
  })

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      const params: ListAuditLogsParams = { page, page_size: pageSize }
      if (filters.userId) params.user_id = filters.userId
      if (filters.actorId) params.actor_id = filters.actorId
      if (filters.eventType) params.event_type = filters.eventType
      if (filters.fromDate) params.from_date = new Date(filters.fromDate).toISOString()
      if (filters.toDate) params.to_date = new Date(filters.toDate).toISOString()
      if (filters.success === 'true') params.success = true
      if (filters.success === 'false') params.success = false

      const res = await auditApi.list(params)
      setData({ items: res.data, total: res.total, totalPages: res.total_pages })
    } catch {
      toast({ title: 'Error al cargar logs de auditoría', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [page, pageSize, filters])

  useEffect(() => { void load() }, [load])

  const handleFilterChange = (key: keyof typeof filters, value: string) => {
    // When category changes, clear specific event type
    if (key === 'category') {
      setFilters((prev) => ({ ...prev, category: value, eventType: '' }))
    } else {
      setFilters((prev) => ({ ...prev, [key]: value }))
    }
    reset()
  }

  const clearFilters = () => {
    setFilters({ userId: '', actorId: '', category: '', eventType: '', fromDate: '', toDate: '', success: '' })
    reset()
  }

  const activeFiltersCount = Object.values(filters).filter(Boolean).length

  // Events available for the event type dropdown (filtered by category if selected)
  const availableEvents =
    filters.category && filters.category !== '_all'
      ? EVENT_CATEGORIES[filters.category] ?? []
      : Object.values(EVENT_CATEGORIES).flat()

  return (
    <div>
      <PageHeader
        title="Auditoría"
        description="Registro de eventos del sistema (solo lectura)"
        actions={
          <Button
            variant="outline"
            onClick={() => {
              if (data.items.length === 0) {
                toast({ title: 'Sin datos para exportar', variant: 'destructive' })
                return
              }
              exportToCsv(data.items)
            }}
          >
            <Download className="h-4 w-4" />
            Exportar CSV
          </Button>
        }
      />

      {/* Filters */}
      <div
        className="rounded-xl p-4 mb-4 space-y-3"
        style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-sm font-medium" style={{ color: '#374151' }}>
            <Filter className="h-4 w-4" style={{ color: '#9CA3AF' }} aria-hidden="true" />
            Filtros
            {activeFiltersCount > 0 && (
              <span
                className="text-xs px-1.5 py-0.5 rounded-full font-semibold"
                style={{ backgroundColor: '#004899', color: '#FFFFFF' }}
              >
                {activeFiltersCount}
              </span>
            )}
          </div>
          {activeFiltersCount > 0 && (
            <button
              onClick={clearFilters}
              className="text-xs transition-colors"
              style={{ color: '#9CA3AF' }}
              onMouseEnter={(e) => { e.currentTarget.style.color = '#004899' }}
              onMouseLeave={(e) => { e.currentTarget.style.color = '#9CA3AF' }}
            >
              Limpiar filtros
            </button>
          )}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {/* User ID */}
          <div className="space-y-1">
            <Label htmlFor="filter-user-id">ID de usuario</Label>
            <Input
              id="filter-user-id"
              placeholder="UUID del usuario"
              value={filters.userId}
              onChange={(e) => handleFilterChange('userId', e.target.value)}
              className="text-xs font-mono"
            />
          </div>

          {/* Actor ID */}
          <div className="space-y-1">
            <Label htmlFor="filter-actor-id">ID del actor</Label>
            <Input
              id="filter-actor-id"
              placeholder="UUID del actor"
              value={filters.actorId}
              onChange={(e) => handleFilterChange('actorId', e.target.value)}
              className="text-xs font-mono"
            />
          </div>

          {/* Category */}
          <div className="space-y-1">
            <Label>Categoría</Label>
            <Select
              value={filters.category || '_all'}
              onValueChange={(val) => handleFilterChange('category', val === '_all' ? '' : val)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Todas las categorías" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_all">Todas las categorías</SelectItem>
                {Object.keys(EVENT_CATEGORIES).map((cat) => (
                  <SelectItem key={cat} value={cat}>{cat}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Event type */}
          <div className="space-y-1">
            <Label>Tipo de evento</Label>
            <Select
              value={filters.eventType || '_all'}
              onValueChange={(val) => handleFilterChange('eventType', val === '_all' ? '' : val)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Todos los eventos" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_all">Todos los eventos</SelectItem>
                {availableEvents.map((et) => (
                  <SelectItem key={et} value={et}>
                    {eventLabel(et)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Desde */}
          <div className="space-y-1">
            <Label htmlFor="filter-from-date">Desde</Label>
            <Input
              id="filter-from-date"
              type="datetime-local"
              value={filters.fromDate}
              onChange={(e) => handleFilterChange('fromDate', e.target.value)}
            />
          </div>

          {/* Hasta */}
          <div className="space-y-1">
            <Label htmlFor="filter-to-date">Hasta</Label>
            <Input
              id="filter-to-date"
              type="datetime-local"
              value={filters.toDate}
              onChange={(e) => handleFilterChange('toDate', e.target.value)}
            />
          </div>
        </div>

        {/* Result filter pills */}
        <div className="flex items-center gap-2">
          <span className="text-xs" style={{ color: '#6B7280' }}>Resultado:</span>
          {([['', 'Todos'], ['true', 'Éxito'], ['false', 'Fallo']] as [string, string][]).map(([val, label]) => (
            <button
              key={val}
              onClick={() => handleFilterChange('success', val)}
              className="px-3 py-1 rounded-full text-xs font-medium transition-colors"
              style={
                filters.success === val
                  ? { backgroundColor: '#004899', color: '#FFFFFF' }
                  : { backgroundColor: '#F3F4F6', color: '#374151' }
              }
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* Table */}
      <div className="space-y-4">
        <div className="rounded-xl overflow-hidden" style={{ border: '1px solid #E0E5EC' }}>
          <table className="w-full text-sm">
            <thead style={{ backgroundColor: '#F4F6F9', borderBottom: '1px solid #E0E5EC' }}>
              <tr>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                  Timestamp
                </th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                  Evento
                </th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                  Actor
                </th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                  Recurso
                </th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                  IP
                </th>
                <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider" style={{ color: '#6B7280' }}>
                  Resultado
                </th>
              </tr>
            </thead>
            <tbody style={{ backgroundColor: '#FFFFFF' }}>
              {isLoading ? (
                <tr>
                  <td colSpan={6} className="px-4 py-16 text-center" style={{ color: '#9CA3AF' }}>
                    <div className="flex items-center justify-center gap-2">
                      <span
                        className="h-4 w-4 animate-spin rounded-full border-2 border-t-transparent"
                        style={{ borderColor: '#004899', borderTopColor: 'transparent' }}
                      />
                      Cargando...
                    </div>
                  </td>
                </tr>
              ) : data.items.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-16 text-center" style={{ color: '#9CA3AF' }}>
                    No se encontraron eventos de auditoría.
                  </td>
                </tr>
              ) : (
                data.items.map((log) => (
                  <tr
                    key={log.id}
                    className="cursor-pointer transition-colors"
                    style={{ borderTop: '1px solid #F3F4F6' }}
                    onClick={() => setSelectedLog(log)}
                    onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = '#F4F6F9' }}
                    onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent' }}
                    title="Clic para ver detalle"
                  >
                    <td className="px-4 py-3 whitespace-nowrap">
                      <div className="text-xs" style={{ color: '#374151' }}>
                        {formatDate(log.created_at)}
                      </div>
                      <div className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
                        {formatDateRelative(log.created_at)}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={eventBadgeVariant(log.event_type)} className="text-xs whitespace-nowrap">
                        {eventLabel(log.event_type)}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-xs font-mono" style={{ color: '#6B7280' }}>
                        {log.actor_id ? log.actor_id.slice(0, 8) + '…' : (
                          <span style={{ color: '#9CA3AF' }}>—</span>
                        )}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      {log.resource_type ? (
                        <span className="text-xs font-mono" style={{ color: '#374151' }}>
                          {log.resource_type}
                        </span>
                      ) : (
                        <span style={{ color: '#9CA3AF' }}>—</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-xs font-mono" style={{ color: '#6B7280' }}>
                        {log.ip_address ?? <span style={{ color: '#9CA3AF' }}>—</span>}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={log.success ? 'success' : 'destructive'} className="text-xs">
                        {log.success ? 'Éxito' : 'Fallo'}
                      </Badge>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        <div className="flex items-center justify-between text-sm" style={{ color: '#6B7280' }}>
          <span>
            {data.total === 0
              ? 'Sin resultados'
              : `Mostrando ${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, data.total)} de ${data.total} evento${data.total !== 1 ? 's' : ''}`}
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => goToPage(page - 1)}
              disabled={page <= 1 || isLoading}
            >
              Anterior
            </Button>
            <span className="px-2 font-medium" style={{ color: '#374151' }}>
              Página {page} de {Math.max(1, data.totalPages)}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => goToPage(page + 1)}
              disabled={page >= data.totalPages || isLoading}
            >
              Siguiente
            </Button>
          </div>
        </div>
      </div>

      {/* Detail modal */}
      {selectedLog && (
        <DetailModal log={selectedLog} onClose={() => setSelectedLog(null)} />
      )}
    </div>
  )
}

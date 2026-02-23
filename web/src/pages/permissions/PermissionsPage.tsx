import { useEffect, useState, useCallback } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Plus, Trash2, Search, ChevronDown, ChevronRight } from 'lucide-react'
import { permissionsApi } from '@/api/permissions'
import type { Permission, ApiError } from '@/types'
import { PageHeader } from '@/components/shared/PageHeader'
import { ConfirmDialog } from '@/components/shared/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import { toast } from '@/hooks/useToast'
import { useDebounce } from '@/hooks/useDebounce'
import type { AxiosError } from 'axios'

// ─── Types ─────────────────────────────────────────────────────────────────

type ScopeType = 'global' | 'module' | 'resource' | 'action'
type ScopeFilter = 'all' | ScopeType

// ─── Schema ────────────────────────────────────────────────────────────────

const permissionSchema = z.object({
  code: z
    .string()
    .min(1, 'Requerido')
    .regex(
      /^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$/,
      'Formato: modulo.recurso.accion (solo minúsculas, números y _)'
    ),
  description: z.string().optional(),
  scope_type: z.enum(['global', 'module', 'resource', 'action'], {
    error: 'Seleccione un scope',
  }),
})

type PermissionFormData = z.infer<typeof permissionSchema>

// ─── Helpers ───────────────────────────────────────────────────────────────

function getModule(code: string): string {
  return code.split('.')[0] ?? 'general'
}

function parseCode(code: string): { module: string; resource: string; action: string } | null {
  const parts = code.split('.')
  if (parts.length !== 3) return null
  return { module: parts[0], resource: parts[1], action: parts[2] }
}

function groupByModule(permissions: Permission[]): Record<string, Permission[]> {
  return permissions.reduce<Record<string, Permission[]>>((acc, p) => {
    const mod = getModule(p.code)
    if (!acc[mod]) acc[mod] = []
    acc[mod].push(p)
    return acc
  }, {})
}

const scopeColors: Record<ScopeType, { bg: string; text: string; label: string }> = {
  global:   { bg: '#EFF6FF', text: '#004899', label: 'Global' },
  module:   { bg: '#F3F4F6', text: '#374151', label: 'Módulo' },
  resource: { bg: '#FFF7ED', text: '#EA580C', label: 'Recurso' },
  action:   { bg: '#F0FDF4', text: '#16A34A', label: 'Acción' },
}

function ScopePill({ scope }: { scope: string }) {
  const s = scopeColors[scope as ScopeType]
  if (!s) return <Badge variant="secondary" className="text-xs capitalize">{scope}</Badge>
  return (
    <span
      className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium"
      style={{ backgroundColor: s.bg, color: s.text }}
    >
      {s.label}
    </span>
  )
}

// ─── Module accordion section ───────────────────────────────────────────────

interface ModuleSectionProps {
  module: string
  permissions: Permission[]
  onDelete: (p: Permission) => void
}

function ModuleSection({ module, permissions, onDelete }: ModuleSectionProps) {
  const [expanded, setExpanded] = useState(true)

  return (
    <div className="rounded-xl overflow-hidden" style={{ border: '1px solid #E0E5EC' }}>
      {/* Header */}
      <button
        type="button"
        className="w-full flex items-center justify-between px-5 py-3 transition-colors"
        style={{ backgroundColor: '#F9FAFB' }}
        onClick={() => setExpanded((v) => !v)}
        onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#F3F4F6')}
        onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#F9FAFB')}
      >
        <div className="flex items-center gap-2.5">
          {expanded
            ? <ChevronDown className="h-4 w-4" style={{ color: '#6B7280' }} />
            : <ChevronRight className="h-4 w-4" style={{ color: '#6B7280' }} />}
          <span className="font-mono font-semibold text-sm" style={{ color: '#1A1A2E' }}>
            {module}
          </span>
        </div>
        <span
          className="text-xs font-semibold px-2 py-0.5 rounded-full"
          style={{ backgroundColor: '#EFF6FF', color: '#004899' }}
        >
          {permissions.length}
        </span>
      </button>

      {/* Rows */}
      {expanded && (
        <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
          {/* Sub-header */}
          <div
            className="grid grid-cols-[2fr_2fr_1fr_auto] gap-3 px-5 py-2 text-xs font-medium uppercase tracking-wide"
            style={{ color: '#9CA3AF', backgroundColor: '#FAFAFA' }}
          >
            <span>Código</span>
            <span>Descripción</span>
            <span>Scope</span>
            <span />
          </div>

          {permissions.map((perm) => (
            <div
              key={perm.id}
              className="grid grid-cols-[2fr_2fr_1fr_auto] gap-3 px-5 py-3 items-center hover:bg-gray-50/60 transition-colors"
            >
              <span className="font-mono text-sm font-medium truncate" style={{ color: '#1A1A2E' }}>
                {perm.code}
              </span>
              <span className="text-sm truncate" style={{ color: '#6B7280' }}>
                {perm.description ?? <span style={{ color: '#D1D5DB' }}>—</span>}
              </span>
              <ScopePill scope={perm.scope_type} />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => onDelete(perm)}
                aria-label={`Eliminar permiso ${perm.code}`}
                title="Eliminar"
              >
                <Trash2 className="h-4 w-4 text-red-400 hover:text-red-600" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Create dialog ─────────────────────────────────────────────────────────

interface CreateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}

const scopeOptions: { value: ScopeType; label: string; description: string }[] = [
  { value: 'global',   label: 'Global',   description: 'Aplica a todo el sistema' },
  { value: 'module',   label: 'Módulo',   description: 'Aplica a un módulo específico' },
  { value: 'resource', label: 'Recurso',  description: 'Aplica a un tipo de recurso' },
  { value: 'action',   label: 'Acción',   description: 'Aplica a una acción concreta' },
]

function CreateDialog({ open, onOpenChange, onSuccess }: CreateDialogProps) {
  const {
    register,
    handleSubmit,
    reset,
    watch,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<PermissionFormData>({ resolver: zodResolver(permissionSchema) })

  const codeValue = watch('code') ?? ''
  const scopeValue = watch('scope_type')
  const parsed = parseCode(codeValue)

  useEffect(() => {
    if (!open) reset()
  }, [open, reset])

  const onSubmit = async (data: PermissionFormData) => {
    try {
      await permissionsApi.create(data)
      toast({ title: 'Permiso creado', description: `"${data.code}" creado exitosamente.` })
      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      const axiosErr = err as AxiosError<ApiError>
      const msg = axiosErr.response?.data?.error?.message ?? 'No se pudo crear el permiso.'
      toast({ title: 'Error', description: msg, variant: 'destructive' })
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) reset(); onOpenChange(o) }}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Nuevo permiso</DialogTitle>
          <DialogDescription>
            El código debe seguir el formato{' '}
            <code className="text-xs font-mono px-1 py-0.5 rounded" style={{ backgroundColor: '#F3F4F6' }}>
              modulo.recurso.accion
            </code>
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-5">
          {/* Code */}
          <div className="space-y-1.5">
            <Label htmlFor="perm-code">Código *</Label>
            <Input
              id="perm-code"
              placeholder="inventory.stock.read"
              className="font-mono"
              aria-invalid={!!errors.code}
              {...register('code')}
            />
            {errors.code ? (
              <p className="text-xs text-red-600" role="alert">{errors.code.message}</p>
            ) : parsed ? (
              /* Parse preview */
              <div
                className="flex items-center gap-0 rounded-md overflow-hidden text-xs font-mono"
                style={{ border: '1px solid #E0E5EC' }}
              >
                <div className="px-3 py-1.5 flex-1 text-center" style={{ backgroundColor: '#EFF6FF', color: '#004899', borderRight: '1px solid #E0E5EC' }}>
                  <span className="block text-[10px] uppercase tracking-wide font-sans font-semibold mb-0.5" style={{ color: '#9CA3AF' }}>Módulo</span>
                  {parsed.module}
                </div>
                <div className="px-3 py-1.5 flex-1 text-center" style={{ backgroundColor: '#F9FAFB', borderRight: '1px solid #E0E5EC' }}>
                  <span className="block text-[10px] uppercase tracking-wide font-sans font-semibold mb-0.5" style={{ color: '#9CA3AF' }}>Recurso</span>
                  {parsed.resource}
                </div>
                <div className="px-3 py-1.5 flex-1 text-center" style={{ backgroundColor: '#F0FDF4', color: '#16A34A' }}>
                  <span className="block text-[10px] uppercase tracking-wide font-sans font-semibold mb-0.5" style={{ color: '#9CA3AF' }}>Acción</span>
                  {parsed.action}
                </div>
              </div>
            ) : codeValue ? (
              <p className="text-xs" style={{ color: '#9CA3AF' }}>
                Escriba el formato completo: <code>modulo.recurso.accion</code>
              </p>
            ) : null}
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <Label htmlFor="perm-description">Descripción (opcional)</Label>
            <Input
              id="perm-description"
              placeholder="Permite ver registros de stock..."
              {...register('description')}
            />
          </div>

          {/* Scope — radio buttons */}
          <div className="space-y-2">
            <Label>Scope *</Label>
            <div className="grid grid-cols-2 gap-2">
              {scopeOptions.map(({ value, label, description }) => {
                const isSelected = scopeValue === value
                const colors = scopeColors[value]
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setValue('scope_type', value, { shouldValidate: true })}
                    className="flex flex-col items-start rounded-lg px-3 py-2.5 transition-colors text-left"
                    style={{
                      border: `2px solid ${isSelected ? colors.text : '#E0E5EC'}`,
                      backgroundColor: isSelected ? colors.bg : '#FFFFFF',
                    }}
                  >
                    <span className="text-sm font-medium" style={{ color: isSelected ? colors.text : '#374151' }}>
                      {label}
                    </span>
                    <span className="text-xs mt-0.5" style={{ color: '#9CA3AF' }}>
                      {description}
                    </span>
                  </button>
                )
              })}
            </div>
            {errors.scope_type && (
              <p className="text-xs text-red-600" role="alert">{errors.scope_type.message}</p>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => { reset(); onOpenChange(false) }}
              disabled={isSubmitting}
            >
              Cancelar
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? 'Creando...' : 'Crear permiso'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ─── Main page ──────────────────────────────────────────────────────────────

export function PermissionsPage() {
  const [allPermissions, setAllPermissions] = useState<Permission[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const [deletePermission, setDeletePermission] = useState<Permission | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [scopeFilter, setScopeFilter] = useState<ScopeFilter>('all')
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 200)

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      // Fetch up to 200 permissions for grouped view
      const res = await permissionsApi.list({ page: 1, page_size: 200 })
      setAllPermissions(res.data)
    } catch {
      toast({ title: 'Error al cargar permisos', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const handleDelete = async () => {
    if (!deletePermission) return
    setIsDeleting(true)
    try {
      await permissionsApi.delete(deletePermission.id)
      toast({ title: 'Permiso eliminado', description: `"${deletePermission.code}" eliminado.` })
      setDeletePermission(null)
      void load()
    } catch {
      toast({ title: 'Error al eliminar el permiso', variant: 'destructive' })
    } finally {
      setIsDeleting(false)
    }
  }

  // Filter + search
  const filtered = allPermissions.filter((p) => {
    const matchScope = scopeFilter === 'all' || p.scope_type === scopeFilter
    const matchSearch = !debouncedSearch || p.code.toLowerCase().includes(debouncedSearch.toLowerCase())
    return matchScope && matchSearch
  })

  const grouped = groupByModule(filtered)
  const modules = Object.keys(grouped).sort()

  const scopeFilterOptions: { label: string; value: ScopeFilter }[] = [
    { label: 'Todos', value: 'all' },
    { label: 'Global', value: 'global' },
    { label: 'Módulo', value: 'module' },
    { label: 'Recurso', value: 'resource' },
    { label: 'Acción', value: 'action' },
  ]

  return (
    <div>
      <PageHeader
        title="Permisos"
        description={`${allPermissions.length} permisos registrados en el sistema`}
        actions={
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4" />
            Nuevo permiso
          </Button>
        }
      />

      {/* Filters row */}
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        {/* Search */}
        <div className="relative max-w-xs">
          <Search
            className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 pointer-events-none"
            style={{ color: '#9CA3AF' }}
          />
          <Input
            placeholder="Buscar por código..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
            aria-label="Buscar permisos"
          />
        </div>

        {/* Scope filter */}
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm" style={{ color: '#6B7280' }}>Scope:</span>
          {scopeFilterOptions.map(({ label, value }) => (
            <button
              key={value}
              onClick={() => setScopeFilter(value)}
              className="px-3 py-1 rounded-full text-xs font-medium transition-colors"
              style={
                scopeFilter === value
                  ? { backgroundColor: '#004899', color: '#FFFFFF' }
                  : { backgroundColor: '#F3F4F6', color: '#374151' }
              }
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* Grouped view */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-24 animate-pulse rounded-xl" style={{ backgroundColor: '#F3F4F6' }} />
          ))}
        </div>
      ) : modules.length === 0 ? (
        <div
          className="rounded-xl py-16 text-center text-sm"
          style={{ border: '1px solid #E0E5EC', color: '#9CA3AF' }}
        >
          {debouncedSearch || scopeFilter !== 'all'
            ? 'No hay permisos que coincidan con los filtros.'
            : 'No hay permisos registrados.'}
        </div>
      ) : (
        <div className="space-y-3">
          {modules.map((mod) => (
            <ModuleSection
              key={mod}
              module={mod}
              permissions={grouped[mod]}
              onDelete={setDeletePermission}
            />
          ))}
        </div>
      )}

      {/* Summary footer */}
      {!isLoading && filtered.length > 0 && (
        <p className="text-xs mt-4 text-right" style={{ color: '#9CA3AF' }}>
          {filtered.length} permiso(s) en {modules.length} módulo(s)
        </p>
      )}

      <CreateDialog open={showCreate} onOpenChange={setShowCreate} onSuccess={load} />

      {deletePermission && (
        <ConfirmDialog
          open
          onOpenChange={(open) => { if (!open) setDeletePermission(null) }}
          title="Eliminar permiso"
          description={`¿Eliminar el permiso "${deletePermission.code}"? Esta acción es irreversible y afectará todos los roles y usuarios que lo tengan asignado.`}
          confirmLabel="Eliminar"
          variant="destructive"
          onConfirm={handleDelete}
          isLoading={isDeleting}
        />
      )}
    </div>
  )
}

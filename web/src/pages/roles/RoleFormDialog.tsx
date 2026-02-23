import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ChevronDown, ChevronRight } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { rolesApi } from '@/api/roles'
import { permissionsApi } from '@/api/permissions'
import { toast } from '@/hooks/useToast'
import type { Role, Permission, ApiError } from '@/types'
import type { AxiosError } from 'axios'

// ─── Schema ────────────────────────────────────────────────────────────────

const roleSchema = z.object({
  name: z.string().min(1, 'El nombre es requerido').max(100),
  description: z.string().optional(),
})

type RoleFormData = z.infer<typeof roleSchema>

// ─── Permission grouping ───────────────────────────────────────────────────

function getModule(code: string): string {
  const parts = code.split('.')
  return parts[0] ?? 'general'
}

function groupByModule(permissions: Permission[]): Record<string, Permission[]> {
  return permissions.reduce<Record<string, Permission[]>>((acc, p) => {
    const mod = getModule(p.code)
    if (!acc[mod]) acc[mod] = []
    acc[mod].push(p)
    return acc
  }, {})
}

// ─── Permission accordion section ─────────────────────────────────────────

interface ModuleSectionProps {
  module: string
  permissions: Permission[]
  selected: string[]
  onToggle: (id: string) => void
}

function ModuleSection({ module, permissions, selected, onToggle }: ModuleSectionProps) {
  const [expanded, setExpanded] = useState(true)
  const checkedCount = permissions.filter((p) => selected.includes(p.id)).length

  return (
    <div className="rounded-md overflow-hidden" style={{ border: '1px solid #E0E5EC' }}>
      {/* Header */}
      <button
        type="button"
        className="w-full flex items-center justify-between px-3 py-2 text-xs font-semibold uppercase tracking-wide transition-colors"
        style={{ backgroundColor: '#F9FAFB', color: '#374151' }}
        onClick={() => setExpanded((v) => !v)}
        onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#F3F4F6')}
        onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#F9FAFB')}
      >
        <div className="flex items-center gap-2">
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          {module}
        </div>
        {checkedCount > 0 && (
          <span
            className="px-1.5 py-0.5 rounded-full text-xs font-semibold"
            style={{ backgroundColor: '#EFF6FF', color: '#004899' }}
          >
            {checkedCount}
          </span>
        )}
      </button>

      {/* Items */}
      {expanded && (
        <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
          {permissions.map((p) => (
            <label
              key={p.id}
              className="flex items-center gap-2.5 px-3 py-2 cursor-pointer transition-colors"
              style={{ backgroundColor: selected.includes(p.id) ? '#EFF6FF' : undefined }}
              onMouseEnter={(e) => {
                if (!selected.includes(p.id))
                  (e.currentTarget as HTMLElement).style.backgroundColor = '#FAFAFA'
              }}
              onMouseLeave={(e) => {
                if (!selected.includes(p.id))
                  (e.currentTarget as HTMLElement).style.backgroundColor = ''
              }}
            >
              <input
                type="checkbox"
                checked={selected.includes(p.id)}
                onChange={() => onToggle(p.id)}
                className="h-3.5 w-3.5 rounded border-gray-300 shrink-0"
                style={{ accentColor: '#004899' }}
              />
              <div className="min-w-0">
                <span className="font-mono text-xs" style={{ color: '#1A1A2E' }}>{p.code}</span>
                {p.description && (
                  <span className="block text-xs truncate" style={{ color: '#9CA3AF' }}>
                    {p.description}
                  </span>
                )}
              </div>
            </label>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Dialog ────────────────────────────────────────────────────────────────

interface RoleFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
  role?: Role | null
}

export function RoleFormDialog({ open, onOpenChange, onSuccess, role }: RoleFormDialogProps) {
  const isEditing = !!role
  const [allPermissions, setAllPermissions] = useState<Permission[]>([])
  const [selectedPermIds, setSelectedPermIds] = useState<string[]>([])
  const [loadingPerms, setLoadingPerms] = useState(false)

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<RoleFormData>({
    resolver: zodResolver(roleSchema),
    defaultValues: { name: '', description: '' },
  })

  useEffect(() => {
    if (!open) return

    reset({ name: role?.name ?? '', description: role?.description ?? '' })
    setSelectedPermIds([])

    // Load permissions
    setLoadingPerms(true)
    const permLoad = permissionsApi.list({ page_size: 100 }).then((r) => {
      setAllPermissions(r.data)
      return r.data
    })

    // If editing, also fetch role with permissions to pre-select
    if (isEditing && role) {
      void permLoad.then((perms) => {
        rolesApi.get(role.id).then((fullRole) => {
          const assignedIds = new Set(fullRole.permissions?.map((p) => p.id) ?? [])
          setSelectedPermIds(perms.filter((p) => assignedIds.has(p.id)).map((p) => p.id))
        }).catch(() => {/* ignore */})
      })
    }

    void permLoad.finally(() => setLoadingPerms(false))
  }, [open, role, isEditing, reset])

  const togglePerm = (id: string) => {
    setSelectedPermIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
  }

  const onSubmit = async (data: RoleFormData) => {
    try {
      let targetId: string

      if (isEditing && role) {
        await rolesApi.update(role.id, data)
        targetId = role.id
        toast({ title: 'Rol actualizado', description: `"${data.name}" actualizado.` })
      } else {
        const created = await rolesApi.create(data)
        targetId = created.id
        toast({ title: 'Rol creado', description: `"${data.name}" creado exitosamente.` })
      }

      // Assign selected permissions if any
      if (selectedPermIds.length > 0) {
        await rolesApi.assignPermissions(targetId, { permission_ids: selectedPermIds })
      }

      reset()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      const axiosErr = err as AxiosError<ApiError>
      const msg = axiosErr.response?.data?.error?.message ?? 'No se pudo guardar el rol.'
      toast({ title: 'Error', description: msg, variant: 'destructive' })
    }
  }

  const grouped = groupByModule(allPermissions)
  const modules = Object.keys(grouped).sort()

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) reset(); onOpenChange(o) }}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Editar rol' : 'Nuevo rol'}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Modifique los datos del rol.'
              : 'Complete los datos para crear un nuevo rol de acceso.'}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-5">
          {/* Name */}
          <div className="space-y-1.5">
            <Label htmlFor="role-name">Nombre *</Label>
            <Input
              id="role-name"
              placeholder="supervisor"
              aria-invalid={!!errors.name}
              disabled={isEditing && role?.is_system}
              {...register('name')}
            />
            {errors.name && (
              <p className="text-xs text-red-600" role="alert">{errors.name.message}</p>
            )}
            {isEditing && role?.is_system && (
              <p className="text-xs text-amber-600">Los roles del sistema no pueden cambiar nombre.</p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <Label htmlFor="role-description">Descripción (opcional)</Label>
            <Input
              id="role-description"
              placeholder="Descripción del rol..."
              {...register('description')}
            />
          </div>

          {/* Permissions */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>
                Permisos
                {selectedPermIds.length > 0 && (
                  <span className="ml-2 text-xs font-normal" style={{ color: '#004899' }}>
                    {selectedPermIds.length} seleccionado(s)
                  </span>
                )}
              </Label>
              {selectedPermIds.length > 0 && (
                <button
                  type="button"
                  className="text-xs"
                  style={{ color: '#6B7280' }}
                  onClick={() => setSelectedPermIds([])}
                >
                  Limpiar
                </button>
              )}
            </div>

            {loadingPerms ? (
              <div className="space-y-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div key={i} className="h-9 animate-pulse rounded-md" style={{ backgroundColor: '#F3F4F6' }} />
                ))}
              </div>
            ) : modules.length === 0 ? (
              <p className="text-xs text-center py-4" style={{ color: '#9CA3AF' }}>
                No hay permisos disponibles.
              </p>
            ) : (
              <div className="space-y-2 max-h-64 overflow-y-auto pr-1">
                {modules.map((mod) => (
                  <ModuleSection
                    key={mod}
                    module={mod}
                    permissions={grouped[mod]}
                    selected={selectedPermIds}
                    onToggle={togglePerm}
                  />
                ))}
              </div>
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
              {isSubmitting
                ? 'Guardando...'
                : isEditing
                ? 'Guardar cambios'
                : 'Crear rol'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

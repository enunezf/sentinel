import { useEffect, useState, useCallback } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import type { ColumnDef } from '@tanstack/react-table'
import { Plus, Pencil, ToggleLeft, ToggleRight, Search } from 'lucide-react'
import { costCentersApi } from '@/api/costCenters'
import { applicationsApi } from '@/api/applications'
import type { CostCenter, Application, ApiError } from '@/types'
import { DataTable } from '@/components/shared/DataTable'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { PageHeader } from '@/components/shared/PageHeader'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import { toast } from '@/hooks/useToast'
import { usePagination } from '@/hooks/usePagination'
import { useDebounce } from '@/hooks/useDebounce'
import { formatDate } from '@/lib/utils'
import type { AxiosError } from 'axios'

// ─── Schemas ───────────────────────────────────────────────────────────────

const createSchema = z.object({
  code: z.string().min(1, 'Requerido').max(50),
  name: z.string().min(1, 'Requerido').max(200),
})

const editSchema = z.object({
  name: z.string().min(1, 'Requerido').max(200),
  is_active: z.boolean(),
})

type CreateFormData = z.infer<typeof createSchema>
type EditFormData = z.infer<typeof editSchema>

type StatusFilter = 'all' | 'active' | 'inactive'

// ─── Page ───────────────────────────────────────────────────────────────────

export function CostCentersPage() {
  const { page, pageSize, goToPage, reset } = usePagination()
  const [data, setData] = useState<{ items: CostCenter[]; total: number; totalPages: number }>({
    items: [],
    total: 0,
    totalPages: 0,
  })
  const [isLoading, setIsLoading] = useState(false)

  // Filters
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [appFilter, setAppFilter] = useState<string>('all')
  const [applications, setApplications] = useState<Application[]>([])

  // Dialogs
  const [showCreate, setShowCreate] = useState(false)
  const [editCC, setEditCC] = useState<CostCenter | null>(null)

  const [isTogglingId, setIsTogglingId] = useState<string | null>(null)

  const createForm = useForm<CreateFormData>({ resolver: zodResolver(createSchema) })
  const editForm = useForm<EditFormData>({
    resolver: zodResolver(editSchema),
    defaultValues: { name: '', is_active: true },
  })

  // Load applications for filter dropdown
  useEffect(() => {
    void applicationsApi.list({ page_size: 50, is_active: true }).then((r) => setApplications(r.data))
  }, [])

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      const params: Parameters<typeof costCentersApi.list>[0] = {
        page,
        page_size: pageSize,
      }
      if (appFilter !== 'all') params.application_id = appFilter

      const res = await costCentersApi.list(params)
      setData({ items: res.data, total: res.total, totalPages: res.total_pages })
    } catch {
      toast({ title: 'Error al cargar centros de costo', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [page, pageSize, appFilter])

  useEffect(() => { void load() }, [load])

  // Client-side filters: search + status
  const filtered = data.items.filter((cc) => {
    const q = debouncedSearch.toLowerCase()
    const matchSearch =
      !q || cc.code.toLowerCase().includes(q) || cc.name.toLowerCase().includes(q)
    const matchStatus =
      statusFilter === 'all' ||
      (statusFilter === 'active' && cc.is_active) ||
      (statusFilter === 'inactive' && !cc.is_active)
    return matchSearch && matchStatus
  })

  const onCreateSubmit = async (formData: CreateFormData) => {
    try {
      await costCentersApi.create(formData)
      toast({
        title: 'Centro de costo creado',
        description: `"${formData.code} — ${formData.name}" creado.`,
      })
      createForm.reset()
      setShowCreate(false)
      void load()
    } catch (err) {
      const axiosErr = err as AxiosError<ApiError>
      const msg = axiosErr.response?.data?.error?.message ?? 'No se pudo crear el centro de costo.'
      toast({ title: 'Error', description: msg, variant: 'destructive' })
    }
  }

  const onEditSubmit = async (formData: EditFormData) => {
    if (!editCC) return
    try {
      await costCentersApi.update(editCC.id, { name: formData.name, is_active: formData.is_active })
      toast({ title: 'Centro de costo actualizado', description: `"${editCC.code}" actualizado.` })
      editForm.reset()
      setEditCC(null)
      void load()
    } catch {
      toast({ title: 'Error al actualizar', variant: 'destructive' })
    }
  }

  const handleToggleActive = async (cc: CostCenter) => {
    setIsTogglingId(cc.id)
    try {
      await costCentersApi.update(cc.id, { is_active: !cc.is_active })
      toast({
        title: cc.is_active ? 'Centro de costo desactivado' : 'Centro de costo activado',
        description: `"${cc.code} — ${cc.name}"`,
      })
      void load()
    } catch {
      toast({ title: 'Error al cambiar el estado', variant: 'destructive' })
    } finally {
      setIsTogglingId(null)
    }
  }

  const openEdit = (cc: CostCenter) => {
    setEditCC(cc)
    editForm.reset({ name: cc.name, is_active: cc.is_active })
  }

  const columns: ColumnDef<CostCenter>[] = [
    {
      id: 'info',
      header: 'Centro de Costo',
      cell: ({ row }) => {
        const cc = row.original
        return (
          <div className="flex items-center gap-3">
            <span
              className="font-mono text-xs font-semibold px-2 py-1 rounded shrink-0"
              style={{ backgroundColor: '#F3F4F6', color: '#374151' }}
            >
              {cc.code}
            </span>
            <span className="text-sm" style={{ color: '#1A1A2E' }}>{cc.name}</span>
          </div>
        )
      },
    },
    {
      accessorKey: 'is_active',
      header: 'Estado',
      cell: ({ row }) => <StatusBadge active={row.original.is_active} />,
    },
    {
      accessorKey: 'created_at',
      header: 'Creado',
      cell: ({ row }) => (
        <span className="text-xs" style={{ color: '#9CA3AF' }}>
          {formatDate(row.original.created_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => {
        const cc = row.original
        const isToggling = isTogglingId === cc.id
        return (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              title="Editar"
              onClick={() => openEdit(cc)}
              aria-label={`Editar ${cc.name}`}
            >
              <Pencil className="h-4 w-4 text-sodexo-blue" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={cc.is_active ? 'Desactivar' : 'Activar'}
              onClick={() => handleToggleActive(cc)}
              disabled={isToggling}
              aria-label={cc.is_active ? `Desactivar ${cc.name}` : `Activar ${cc.name}`}
            >
              {isToggling ? (
                <span
                  className="h-4 w-4 animate-spin rounded-full border-2 border-t-transparent block"
                  style={{ borderColor: '#004899', borderTopColor: 'transparent' }}
                />
              ) : cc.is_active ? (
                <ToggleRight className="h-4 w-4 text-green-600" />
              ) : (
                <ToggleLeft className="h-4 w-4 text-gray-400" />
              )}
            </Button>
          </div>
        )
      },
    },
  ]

  const statusOptions: { label: string; value: StatusFilter }[] = [
    { label: 'Todos', value: 'all' },
    { label: 'Activos', value: 'active' },
    { label: 'Inactivos', value: 'inactive' },
  ]

  return (
    <div>
      <PageHeader
        title="Centros de Costo"
        description="Gestión de centros de costo del sistema"
        actions={
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4" />
            Nuevo CeCo
          </Button>
        }
      />

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-3 mb-4 flex-wrap">
        {/* Search */}
        <div className="relative max-w-xs">
          <Search
            className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 pointer-events-none"
            style={{ color: '#9CA3AF' }}
            aria-hidden="true"
          />
          <Input
            placeholder="Buscar por código o nombre..."
            value={search}
            onChange={(e) => { setSearch(e.target.value); reset() }}
            className="pl-9"
            aria-label="Buscar centros de costo"
          />
        </div>

        {/* Application filter */}
        {applications.length > 0 && (
          <div className="w-48">
            <Select
              value={appFilter}
              onValueChange={(v) => { setAppFilter(v); reset() }}
            >
              <SelectTrigger aria-label="Filtrar por aplicación">
                <SelectValue placeholder="Aplicación" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Todas las apps</SelectItem>
                {applications.map((app) => (
                  <SelectItem key={app.id} value={app.id}>
                    {app.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* Status filter */}
        <div className="flex items-center gap-2">
          <span className="text-sm" style={{ color: '#6B7280' }}>Estado:</span>
          {statusOptions.map(({ label, value }) => (
            <button
              key={value}
              onClick={() => { setStatusFilter(value); reset() }}
              className="px-3 py-1 rounded-full text-xs font-medium transition-colors"
              style={
                statusFilter === value
                  ? { backgroundColor: '#004899', color: '#FFFFFF' }
                  : { backgroundColor: '#F3F4F6', color: '#374151' }
              }
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <DataTable
        data={filtered}
        columns={columns}
        page={page}
        pageSize={pageSize}
        total={data.total}
        totalPages={data.totalPages}
        onPageChange={goToPage}
        isLoading={isLoading}
        emptyMessage="No se encontraron centros de costo."
      />

      {/* ── Create dialog ─────────────────────────────────────────── */}
      <Dialog
        open={showCreate}
        onOpenChange={(open) => { if (!open) createForm.reset(); setShowCreate(open) }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Nuevo centro de costo</DialogTitle>
            <DialogDescription>
              Ingrese el código y nombre del nuevo centro de costo.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={createForm.handleSubmit(onCreateSubmit)} noValidate className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="cc-code">Código *</Label>
              <Input
                id="cc-code"
                placeholder="CC001"
                className="font-mono uppercase"
                aria-invalid={!!createForm.formState.errors.code}
                {...createForm.register('code')}
              />
              {createForm.formState.errors.code && (
                <p className="text-xs text-red-600" role="alert">
                  {createForm.formState.errors.code.message}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cc-name">Nombre *</Label>
              <Input
                id="cc-name"
                placeholder="Casino Central"
                aria-invalid={!!createForm.formState.errors.name}
                {...createForm.register('name')}
              />
              {createForm.formState.errors.name && (
                <p className="text-xs text-red-600" role="alert">
                  {createForm.formState.errors.name.message}
                </p>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => { createForm.reset(); setShowCreate(false) }}
                disabled={createForm.formState.isSubmitting}
              >
                Cancelar
              </Button>
              <Button type="submit" disabled={createForm.formState.isSubmitting}>
                {createForm.formState.isSubmitting ? 'Creando...' : 'Crear centro de costo'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* ── Edit dialog ───────────────────────────────────────────── */}
      <Dialog open={!!editCC} onOpenChange={(open) => { if (!open) setEditCC(null) }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Editar centro de costo</DialogTitle>
            <DialogDescription>
              Código:{' '}
              <span className="font-mono font-semibold" style={{ color: '#1A1A2E' }}>
                {editCC?.code}
              </span>
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={editForm.handleSubmit(onEditSubmit)} noValidate className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="edit-cc-name">Nombre *</Label>
              <Input
                id="edit-cc-name"
                aria-invalid={!!editForm.formState.errors.name}
                {...editForm.register('name')}
              />
              {editForm.formState.errors.name && (
                <p className="text-xs text-red-600" role="alert">
                  {editForm.formState.errors.name.message}
                </p>
              )}
            </div>

            <div className="flex items-center gap-3">
              <input
                id="edit-cc-active"
                type="checkbox"
                className="h-4 w-4 rounded border-gray-300"
                style={{ accentColor: '#004899' }}
                {...editForm.register('is_active')}
              />
              <Label htmlFor="edit-cc-active">Centro de costo activo</Label>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setEditCC(null)}
                disabled={editForm.formState.isSubmitting}
              >
                Cancelar
              </Button>
              <Button type="submit" disabled={editForm.formState.isSubmitting}>
                {editForm.formState.isSubmitting ? 'Guardando...' : 'Guardar cambios'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

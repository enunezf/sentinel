import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { Plus, Eye, Pencil, Trash2 } from 'lucide-react'
import { rolesApi } from '@/api/roles'
import type { Role } from '@/types'
import { DataTable } from '@/components/shared/DataTable'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { PageHeader } from '@/components/shared/PageHeader'
import { ConfirmDialog } from '@/components/shared/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { RoleFormDialog } from './RoleFormDialog'
import { toast } from '@/hooks/useToast'
import { usePagination } from '@/hooks/usePagination'
import { formatDate } from '@/lib/utils'

type StatusFilter = 'all' | 'active' | 'inactive'

export function RolesPage() {
  const navigate = useNavigate()
  const { page, pageSize, goToPage, reset } = usePagination()
  const [allItems, setAllItems] = useState<Role[]>([])
  const [total, setTotal] = useState(0)
  const [totalPages, setTotalPages] = useState(0)
  const [isLoading, setIsLoading] = useState(false)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [editRole, setEditRole] = useState<Role | null>(null)
  const [deleteRole, setDeleteRole] = useState<Role | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await rolesApi.list({ page, page_size: pageSize })
      setAllItems(res.data)
      setTotal(res.total)
      setTotalPages(res.total_pages)
    } catch {
      toast({ title: 'Error al cargar roles', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [page, pageSize])

  useEffect(() => { void load() }, [load])

  const handleDelete = async () => {
    if (!deleteRole) return
    setIsDeleting(true)
    try {
      await rolesApi.deactivate(deleteRole.id)
      toast({ title: 'Rol desactivado', description: `"${deleteRole.name}" desactivado.` })
      setDeleteRole(null)
      void load()
    } catch {
      toast({ title: 'Error al desactivar el rol', variant: 'destructive' })
    } finally {
      setIsDeleting(false)
    }
  }

  // Client-side status filter
  const filtered = allItems.filter((r) => {
    if (statusFilter === 'active') return r.is_active
    if (statusFilter === 'inactive') return !r.is_active
    return true
  })

  const statusOptions: { label: string; value: StatusFilter }[] = [
    { label: 'Todos', value: 'all' },
    { label: 'Activos', value: 'active' },
    { label: 'Inactivos', value: 'inactive' },
  ]

  const columns: ColumnDef<Role>[] = [
    {
      id: 'name',
      header: 'Rol',
      cell: ({ row }) => {
        const role = row.original
        return (
          <div className="flex items-center gap-3">
            <div
              className="h-8 w-8 rounded flex items-center justify-center shrink-0 text-white text-xs font-bold"
              style={{ backgroundColor: '#7C3AED' }}
            >
              {role.name.charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-sm" style={{ color: '#1A1A2E' }}>
                  {role.name}
                </span>
                {role.is_system && (
                  <Badge variant="system" className="text-xs">Sistema</Badge>
                )}
              </div>
              {role.description && (
                <span className="text-xs truncate" style={{ color: '#6B7280' }}>
                  {role.description}
                </span>
              )}
            </div>
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
      accessorKey: 'permissions_count',
      header: 'Permisos',
      cell: ({ row }) => (
        <span className="text-sm font-mono" style={{ color: '#374151' }}>
          {row.original.permissions_count ?? 0}
        </span>
      ),
    },
    {
      accessorKey: 'users_count',
      header: 'Usuarios',
      cell: ({ row }) => (
        <span className="text-sm font-mono" style={{ color: '#374151' }}>
          {row.original.users_count ?? 0}
        </span>
      ),
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
        const role = row.original
        return (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              title="Ver detalle"
              onClick={() => navigate(`/roles/${role.id}`)}
              aria-label={`Ver detalle de ${role.name}`}
            >
              <Eye className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title="Editar"
              onClick={() => setEditRole(role)}
              aria-label={`Editar ${role.name}`}
            >
              <Pencil className="h-4 w-4 text-sodexo-blue" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={role.is_system ? 'No se puede desactivar un rol del sistema' : 'Desactivar'}
              disabled={role.is_system}
              onClick={() => setDeleteRole(role)}
              aria-label={`Desactivar ${role.name}`}
            >
              <Trash2 className={`h-4 w-4 ${role.is_system ? 'text-gray-300' : 'text-red-500'}`} />
            </Button>
          </div>
        )
      },
    },
  ]

  return (
    <div>
      <PageHeader
        title="Roles"
        description="Gestión de roles de acceso del sistema"
        actions={
          <Button onClick={() => setShowCreateDialog(true)}>
            <Plus className="h-4 w-4" />
            Nuevo rol
          </Button>
        }
      />

      {/* Status filter */}
      <div className="flex items-center gap-2 mb-4">
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

      <DataTable
        data={filtered}
        columns={columns}
        page={page}
        pageSize={pageSize}
        total={total}
        totalPages={totalPages}
        onPageChange={goToPage}
        isLoading={isLoading}
        emptyMessage="No se encontraron roles."
      />

      <RoleFormDialog
        open={showCreateDialog}
        onOpenChange={setShowCreateDialog}
        onSuccess={load}
      />

      <RoleFormDialog
        open={!!editRole}
        onOpenChange={(open) => { if (!open) setEditRole(null) }}
        onSuccess={load}
        role={editRole}
      />

      {deleteRole && (
        <ConfirmDialog
          open
          onOpenChange={(open) => { if (!open) setDeleteRole(null) }}
          title="Desactivar rol"
          description={`¿Desactivar el rol "${deleteRole.name}"? Los usuarios con este rol perderán los accesos asociados.`}
          confirmLabel="Desactivar"
          variant="destructive"
          onConfirm={handleDelete}
          isLoading={isDeleting}
        />
      )}
    </div>
  )
}

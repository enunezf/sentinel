import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { Plus, Eye, Pencil, Search } from 'lucide-react'
import { applicationsApi } from '@/api/applications'
import type { Application } from '@/types'
import { DataTable } from '@/components/shared/DataTable'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { PageHeader } from '@/components/shared/PageHeader'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { ApplicationFormDialog } from './ApplicationFormDialog'
import { toast } from '@/hooks/useToast'
import { usePagination } from '@/hooks/usePagination'
import { formatDate } from '@/lib/utils'
import { useDebounce } from '@/hooks/useDebounce'

export function ApplicationsPage() {
  const navigate = useNavigate()
  const { page, pageSize, goToPage } = usePagination()
  const [data, setData] = useState<{ items: Application[]; total: number; totalPages: number }>({
    items: [],
    total: 0,
    totalPages: 0,
  })
  const [isLoading, setIsLoading] = useState(false)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [editApp, setEditApp] = useState<Application | null>(null)

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await applicationsApi.list({
        page,
        page_size: pageSize,
        ...(debouncedSearch ? { search: debouncedSearch } : {}),
      })
      setData({ items: res.data, total: res.total, totalPages: res.total_pages })
    } catch {
      toast({ title: 'Error al cargar aplicaciones', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [page, pageSize, debouncedSearch])

  useEffect(() => {
    void load()
  }, [load])

  const columns: ColumnDef<Application>[] = [
    {
      id: 'name',
      header: 'Aplicación',
      cell: ({ row }) => {
        const app = row.original
        return (
          <div className="flex items-center gap-3">
            <div
              className="h-8 w-8 rounded flex items-center justify-center shrink-0 text-white font-bold text-xs"
              style={{ backgroundColor: '#004899' }}
            >
              {app.name.charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-sm" style={{ color: '#1A1A2E' }}>
                  {app.name}
                </span>
                {app.is_system && (
                  <Badge variant="system" className="text-xs">Sistema</Badge>
                )}
              </div>
              <span className="text-xs font-mono" style={{ color: '#6B7280' }}>
                {app.slug}
              </span>
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
      accessorKey: 'created_at',
      header: 'Creada',
      cell: ({ row }) => (
        <span className="text-xs" style={{ color: '#9CA3AF' }}>
          {formatDate(row.original.created_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      header: 'Acciones',
      cell: ({ row }) => {
        const app = row.original
        return (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              title="Ver detalle"
              onClick={() => navigate(`/applications/${app.id}`)}
              aria-label={`Ver detalle de ${app.name}`}
            >
              <Eye className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title="Editar"
              onClick={() => setEditApp(app)}
              aria-label={`Editar ${app.name}`}
            >
              <Pencil className="h-4 w-4 text-sodexo-blue" />
            </Button>
          </div>
        )
      },
    },
  ]

  return (
    <div>
      <PageHeader
        title="Aplicaciones"
        description="Gestión de aplicaciones registradas en el sistema"
        actions={
          <Button onClick={() => setShowCreateDialog(true)}>
            <Plus className="h-4 w-4" />
            Nueva aplicación
          </Button>
        }
      />

      {/* Search */}
      <div className="mb-4 relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400 pointer-events-none" />
        <Input
          placeholder="Buscar por nombre o slug..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>

      <DataTable
        data={data.items}
        columns={columns}
        page={page}
        pageSize={pageSize}
        total={data.total}
        totalPages={data.totalPages}
        onPageChange={goToPage}
        isLoading={isLoading}
        emptyMessage="No se encontraron aplicaciones."
      />

      <ApplicationFormDialog
        open={showCreateDialog}
        onOpenChange={setShowCreateDialog}
        onSuccess={load}
      />

      <ApplicationFormDialog
        open={!!editApp}
        onOpenChange={(open) => { if (!open) setEditApp(null) }}
        onSuccess={load}
        app={editApp}
      />
    </div>
  )
}

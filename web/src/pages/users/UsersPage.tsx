import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { Plus, Search, MoreHorizontal, Eye, Pencil, Unlock, RotateCcw, UserX, UserCheck, Shield, Key, Building2 } from 'lucide-react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { usersApi } from '@/api/users'
import type { User } from '@/types'
import { DataTable } from '@/components/shared/DataTable'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { PageHeader } from '@/components/shared/PageHeader'
import { ConfirmDialog } from '@/components/shared/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { UserFormDialog } from './UserFormDialog'
import { AsignarRolModal } from '@/components/users/AsignarRolModal'
import { AsignarPermisoModal } from '@/components/users/AsignarPermisoModal'
import { AsignarCeCosModal } from '@/components/users/AsignarCeCosModal'
import { toast } from '@/hooks/useToast'
import { usePagination } from '@/hooks/usePagination'
import { useDebounce } from '@/hooks/useDebounce'
import { formatDateRelative } from '@/lib/utils'

type ActionModal = 'role' | 'permission' | 'costcenter'
type ConfirmType = 'unlock' | 'reset' | 'activate' | 'deactivate'

export function UsersPage() {
  const navigate = useNavigate()
  const { page, pageSize, goToPage, reset } = usePagination()
  const [data, setData] = useState<{ items: User[]; total: number; totalPages: number }>({
    items: [],
    total: 0,
    totalPages: 0,
  })
  const [isLoading, setIsLoading] = useState(false)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)
  const [filterActive, setFilterActive] = useState<boolean | undefined>(undefined)
  const [showCreateDialog, setShowCreateDialog] = useState(false)

  // Confirm action
  const [confirmAction, setConfirmAction] = useState<{ type: ConfirmType; user: User } | null>(null)
  const [isActionLoading, setIsActionLoading] = useState(false)

  // Assignment modals
  const [assignTarget, setAssignTarget] = useState<{ user: User; modal: ActionModal } | null>(null)

  const load = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await usersApi.list({
        page,
        page_size: pageSize,
        ...(debouncedSearch ? { search: debouncedSearch } : {}),
        ...(filterActive !== undefined ? { is_active: filterActive } : {}),
      })
      setData({ items: res.data, total: res.total, totalPages: res.total_pages })
    } catch {
      toast({ title: 'Error al cargar usuarios', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }, [page, pageSize, debouncedSearch, filterActive])

  useEffect(() => { void load() }, [load])

  const handleSearchChange = (value: string) => {
    setSearch(value)
    reset()
  }

  const handleConfirmAction = async () => {
    if (!confirmAction) return
    setIsActionLoading(true)
    const { type, user } = confirmAction
    try {
      if (type === 'unlock') {
        await usersApi.unlock(user.id)
        toast({ title: 'Usuario desbloqueado', description: `@${user.username} desbloqueado.` })
      } else if (type === 'reset') {
        const res = await usersApi.resetPassword(user.id)
        toast({
          title: 'Contraseña reseteada',
          description: `Contraseña temporal: ${res.temporary_password}`,
        })
      } else if (type === 'activate') {
        await usersApi.update(user.id, { is_active: true })
        toast({ title: 'Usuario activado', description: `@${user.username} activado.` })
      } else {
        await usersApi.update(user.id, { is_active: false })
        toast({ title: 'Usuario desactivado', description: `@${user.username} desactivado.` })
      }
      setConfirmAction(null)
      void load()
    } catch {
      toast({ title: 'Error al ejecutar la acción', variant: 'destructive' })
    } finally {
      setIsActionLoading(false)
    }
  }

  const columns: ColumnDef<User>[] = [
    {
      id: 'user',
      header: 'Usuario',
      cell: ({ row }) => {
        const user = row.original
        return (
          <div className="flex items-center gap-3">
            <div
              className="h-8 w-8 rounded-full flex items-center justify-center shrink-0 text-white text-sm font-semibold"
              style={{ backgroundColor: '#004899' }}
            >
              {user.username.charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-sm" style={{ color: '#1A1A2E' }}>
                  @{user.username}
                </span>
                {user.locked_until && (
                  <Badge variant="destructive" className="text-xs">Bloqueado</Badge>
                )}
              </div>
              <span className="text-xs truncate" style={{ color: '#6B7280' }}>
                {user.email}
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
      id: 'roles',
      header: 'Roles',
      cell: ({ row }) => {
        const count = row.original.roles?.length ?? 0
        return (
          <span className="text-sm font-mono" style={{ color: count > 0 ? '#1A1A2E' : '#9CA3AF' }}>
            {count}
          </span>
        )
      },
    },
    {
      accessorKey: 'last_login_at',
      header: 'Último acceso',
      cell: ({ row }) => (
        <span className="text-xs" style={{ color: '#9CA3AF' }}>
          {row.original.last_login_at
            ? formatDateRelative(row.original.last_login_at)
            : 'Nunca'}
        </span>
      ),
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => {
        const user = row.original
        return (
          <DropdownMenu.Root>
            <DropdownMenu.Trigger asChild>
              <Button variant="ghost" size="icon" aria-label={`Acciones para @${user.username}`}>
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content
                className="z-50 min-w-[180px] rounded-lg shadow-lg py-1 text-sm"
                style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
                align="end"
                sideOffset={4}
              >
                <DropdownItem icon={Eye} label="Ver detalle" onClick={() => navigate(`/users/${user.id}`)} />
                <DropdownItem icon={Pencil} label="Editar usuario" onClick={() => navigate(`/users/${user.id}`)} />
                <DropdownMenu.Separator className="my-1" style={{ height: 1, backgroundColor: '#F3F4F6' }} />
                <DropdownItem icon={Shield} label="Asignar rol" onClick={() => setAssignTarget({ user, modal: 'role' })} />
                <DropdownItem icon={Key} label="Asignar permiso" onClick={() => setAssignTarget({ user, modal: 'permission' })} />
                <DropdownItem icon={Building2} label="Asignar CeCos" onClick={() => setAssignTarget({ user, modal: 'costcenter' })} />
                <DropdownMenu.Separator className="my-1" style={{ height: 1, backgroundColor: '#F3F4F6' }} />
                <DropdownItem
                  icon={RotateCcw}
                  label="Resetear contraseña"
                  onClick={() => setConfirmAction({ type: 'reset', user })}
                />
                {user.locked_until && (
                  <DropdownItem
                    icon={Unlock}
                    label="Desbloquear"
                    onClick={() => setConfirmAction({ type: 'unlock', user })}
                    color="#D97706"
                  />
                )}
                {user.is_active ? (
                  <DropdownItem
                    icon={UserX}
                    label="Desactivar"
                    onClick={() => setConfirmAction({ type: 'deactivate', user })}
                    color="#DC2626"
                  />
                ) : (
                  <DropdownItem
                    icon={UserCheck}
                    label="Activar"
                    onClick={() => setConfirmAction({ type: 'activate', user })}
                    color="#16A34A"
                  />
                )}
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        )
      },
    },
  ]

  const confirmMessages: Record<ConfirmType, { title: string; description: string }> = {
    unlock: {
      title: 'Desbloquear usuario',
      description: `¿Desbloquear la cuenta de @${confirmAction?.user.username}?`,
    },
    reset: {
      title: 'Resetear contraseña',
      description: `Se generará una contraseña temporal para @${confirmAction?.user.username}. Deberá cambiarla en el próximo login.`,
    },
    activate: {
      title: 'Activar usuario',
      description: `¿Activar la cuenta de @${confirmAction?.user.username}?`,
    },
    deactivate: {
      title: 'Desactivar usuario',
      description: `¿Desactivar la cuenta de @${confirmAction?.user.username}? Se revocarán todos sus tokens activos.`,
    },
  }

  return (
    <div>
      <PageHeader
        title="Usuarios"
        description="Gestión de cuentas de usuario del sistema"
        actions={
          <Button onClick={() => setShowCreateDialog(true)}>
            <Plus className="h-4 w-4" />
            Nuevo usuario
          </Button>
        }
      />

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-3 mb-4">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400 pointer-events-none" aria-hidden="true" />
          <Input
            placeholder="Buscar por usuario o email..."
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            className="pl-9"
            aria-label="Buscar usuarios"
          />
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm" style={{ color: '#6B7280' }}>Estado:</span>
          {[
            { label: 'Todos', value: undefined },
            { label: 'Activos', value: true },
            { label: 'Inactivos', value: false },
          ].map(({ label, value }) => (
            <button
              key={label}
              onClick={() => { setFilterActive(value); reset() }}
              className="px-3 py-1 rounded-full text-xs font-medium transition-colors"
              style={
                filterActive === value
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
        data={data.items}
        columns={columns}
        page={page}
        pageSize={pageSize}
        total={data.total}
        totalPages={data.totalPages}
        onPageChange={goToPage}
        isLoading={isLoading}
        emptyMessage="No se encontraron usuarios."
      />

      {/* Dialogs */}
      <UserFormDialog open={showCreateDialog} onOpenChange={setShowCreateDialog} onSuccess={load} />

      {assignTarget?.modal === 'role' && (
        <AsignarRolModal
          open
          onOpenChange={(o) => { if (!o) setAssignTarget(null) }}
          userId={assignTarget.user.id}
          username={assignTarget.user.username}
          onSuccess={() => { setAssignTarget(null); void load() }}
        />
      )}

      {assignTarget?.modal === 'permission' && (
        <AsignarPermisoModal
          open
          onOpenChange={(o) => { if (!o) setAssignTarget(null) }}
          userId={assignTarget.user.id}
          username={assignTarget.user.username}
          onSuccess={() => { setAssignTarget(null); void load() }}
        />
      )}

      {assignTarget?.modal === 'costcenter' && (
        <AsignarCeCosModal
          open
          onOpenChange={(o) => { if (!o) setAssignTarget(null) }}
          userId={assignTarget.user.id}
          username={assignTarget.user.username}
          onSuccess={() => { setAssignTarget(null); void load() }}
        />
      )}

      {confirmAction && (
        <ConfirmDialog
          open
          onOpenChange={(open) => { if (!open) setConfirmAction(null) }}
          title={confirmMessages[confirmAction.type].title}
          description={confirmMessages[confirmAction.type].description}
          confirmLabel="Confirmar"
          variant={confirmAction.type === 'deactivate' || confirmAction.type === 'reset' ? 'destructive' : 'default'}
          onConfirm={handleConfirmAction}
          isLoading={isActionLoading}
        />
      )}
    </div>
  )
}

// ─── Dropdown item helper ───────────────────────────────────────────────────

interface DropdownItemProps {
  icon: React.ComponentType<{ className?: string }>
  label: string
  onClick: () => void
  color?: string
}

function DropdownItem({ icon: Icon, label, onClick, color }: DropdownItemProps) {
  return (
    <DropdownMenu.Item
      className="flex items-center gap-2.5 px-3 py-2 cursor-pointer outline-none transition-colors"
      style={{ color: color ?? '#374151' }}
      onSelect={onClick}
      onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#F9FAFB')}
      onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '')}
    >
      <Icon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span className="text-sm">{label}</span>
    </DropdownMenu.Item>
  )
}

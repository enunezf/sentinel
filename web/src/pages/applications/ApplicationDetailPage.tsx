import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Pencil, Monitor } from 'lucide-react'
import { applicationsApi } from '@/api/applications'
import { rolesApi } from '@/api/roles'
import { permissionsApi } from '@/api/permissions'
import { usersApi } from '@/api/users'
import { costCentersApi } from '@/api/costCenters'
import type { Application, Role, Permission, User, CostCenter } from '@/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ApplicationFormDialog } from './ApplicationFormDialog'
import { toast } from '@/hooks/useToast'
import { formatDate } from '@/lib/utils'
import { cn } from '@/lib/utils'

// ─── Tab types ──────────────────────────────────────────────────────────────

type TabId = 'info' | 'roles' | 'permissions' | 'cost-centers' | 'users'

const tabs: { id: TabId; label: string }[] = [
  { id: 'info', label: 'Información General' },
  { id: 'roles', label: 'Roles' },
  { id: 'permissions', label: 'Permisos' },
  { id: 'cost-centers', label: 'Centros de Costo' },
  { id: 'users', label: 'Usuarios' },
]

// ─── Tab content components ────────────────────────────────────────────────

function LoadingRows({ count = 4 }: { count?: number }) {
  return (
    <>
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="px-5 py-3 flex items-center gap-3">
          <div className="h-4 w-40 animate-pulse rounded" style={{ backgroundColor: '#F3F4F6' }} />
          <div className="h-4 w-24 animate-pulse rounded ml-auto" style={{ backgroundColor: '#F3F4F6' }} />
        </div>
      ))}
    </>
  )
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="px-5 py-10 text-center text-sm" style={{ color: '#9CA3AF' }}>
      {message}
    </div>
  )
}

// Info tab
function InfoTab({ app }: { app: Application }) {
  return (
    <div className="p-5">
      <dl className="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-4 text-sm">
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>ID</dt>
          <dd className="font-mono text-sm break-all" style={{ color: '#374151' }}>{app.id}</dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>Slug</dt>
          <dd className="font-mono text-sm" style={{ color: '#374151' }}>{app.slug}</dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>Estado</dt>
          <dd><StatusBadge active={app.is_active} /></dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>Tipo</dt>
          <dd>
            {app.is_system ? (
              <Badge variant="system">Sistema</Badge>
            ) : (
              <Badge variant="secondary">Normal</Badge>
            )}
          </dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>Creada</dt>
          <dd style={{ color: '#374151' }}>{formatDate(app.created_at)}</dd>
        </div>
        {app.updated_at && (
          <div>
            <dt className="text-xs font-medium uppercase tracking-wide mb-1" style={{ color: '#9CA3AF' }}>Actualizada</dt>
            <dd style={{ color: '#374151' }}>{formatDate(app.updated_at)}</dd>
          </div>
        )}
      </dl>
    </div>
  )
}

// Roles tab
function RolesTab({ appSlug }: { appSlug: string }) {
  const [roles, setRoles] = useState<Role[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    setIsLoading(true)
    rolesApi
      .list({ page: 1, page_size: 100 })
      .then((r) => {
        // Filter roles that belong to this application by name convention
        // (backend doesn't expose application_id on roles list yet — show all)
        setRoles(r.data)
      })
      .catch(() => toast({ title: 'Error al cargar roles', variant: 'destructive' }))
      .finally(() => setIsLoading(false))
  }, [appSlug])

  return (
    <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
      {isLoading ? (
        <LoadingRows />
      ) : roles.length === 0 ? (
        <EmptyState message="No hay roles registrados." />
      ) : (
        roles.map((role) => (
          <div key={role.id} className="px-5 py-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-3 min-w-0">
              <span className="font-medium text-sm truncate" style={{ color: '#1A1A2E' }}>
                {role.name}
              </span>
              {role.is_system && <Badge variant="system" className="text-xs shrink-0">Sistema</Badge>}
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <span className="text-xs" style={{ color: '#6B7280' }}>
                {role.permissions_count ?? 0} permisos
              </span>
              <StatusBadge active={role.is_active} />
            </div>
          </div>
        ))
      )}
    </div>
  )
}

// Permissions tab
function PermissionsTab({ appSlug }: { appSlug: string }) {
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    setIsLoading(true)
    permissionsApi
      .list({ page: 1, page_size: 100 })
      .then((r) => setPermissions(r.data))
      .catch(() => toast({ title: 'Error al cargar permisos', variant: 'destructive' }))
      .finally(() => setIsLoading(false))
  }, [appSlug])

  return (
    <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
      {isLoading ? (
        <LoadingRows />
      ) : permissions.length === 0 ? (
        <EmptyState message="No hay permisos registrados." />
      ) : (
        permissions.map((perm) => (
          <div key={perm.id} className="px-5 py-3 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="font-mono text-sm truncate" style={{ color: '#1A1A2E' }}>
                {perm.code}
              </p>
              {perm.description && (
                <p className="text-xs truncate" style={{ color: '#6B7280' }}>
                  {perm.description}
                </p>
              )}
            </div>
            <Badge variant="secondary" className="text-xs shrink-0">
              {perm.scope_type}
            </Badge>
          </div>
        ))
      )}
    </div>
  )
}

// Cost Centers tab
function CostCentersTab({ appSlug }: { appSlug: string }) {
  const [costCenters, setCostCenters] = useState<CostCenter[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    setIsLoading(true)
    costCentersApi
      .list({ page: 1, page_size: 100 })
      .then((r) => setCostCenters(r.data))
      .catch(() => toast({ title: 'Error al cargar centros de costo', variant: 'destructive' }))
      .finally(() => setIsLoading(false))
  }, [appSlug])

  return (
    <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
      {isLoading ? (
        <LoadingRows />
      ) : costCenters.length === 0 ? (
        <EmptyState message="No hay centros de costo registrados." />
      ) : (
        costCenters.map((cc) => (
          <div key={cc.id} className="px-5 py-3 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="font-medium text-sm" style={{ color: '#1A1A2E' }}>{cc.name}</p>
              <p className="font-mono text-xs" style={{ color: '#6B7280' }}>{cc.code}</p>
            </div>
            <StatusBadge active={cc.is_active} />
          </div>
        ))
      )}
    </div>
  )
}

// Users tab
function UsersTab({ appSlug }: { appSlug: string }) {
  const [users, setUsers] = useState<User[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    setIsLoading(true)
    usersApi
      .list({ page: 1, page_size: 50 })
      .then((r) => setUsers(r.data))
      .catch(() => toast({ title: 'Error al cargar usuarios', variant: 'destructive' }))
      .finally(() => setIsLoading(false))
  }, [appSlug])

  return (
    <div className="divide-y" style={{ borderColor: '#F3F4F6' }}>
      {isLoading ? (
        <LoadingRows />
      ) : users.length === 0 ? (
        <EmptyState message="No hay usuarios registrados." />
      ) : (
        users.map((user) => (
          <div key={user.id} className="px-5 py-3 flex items-center justify-between gap-3">
            <div className="flex items-center gap-3 min-w-0">
              <div
                className="h-7 w-7 rounded-full flex items-center justify-center shrink-0 text-white text-xs font-semibold"
                style={{ backgroundColor: '#004899' }}
              >
                {user.username.charAt(0).toUpperCase()}
              </div>
              <div className="min-w-0">
                <p className="text-sm font-medium truncate" style={{ color: '#1A1A2E' }}>{user.username}</p>
                <p className="text-xs truncate" style={{ color: '#6B7280' }}>{user.email}</p>
              </div>
            </div>
            <StatusBadge active={user.is_active} />
          </div>
        ))
      )}
    </div>
  )
}

// ─── Main Detail Page ───────────────────────────────────────────────────────

export function ApplicationDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [app, setApp] = useState<Application | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<TabId>('info')
  const [showEdit, setShowEdit] = useState(false)

  const loadApp = async () => {
    if (!id) return
    setIsLoading(true)
    try {
      const a = await applicationsApi.get(id)
      setApp(a)
    } catch {
      toast({ title: 'Error al cargar la aplicación', variant: 'destructive' })
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    void loadApp()
  }, [id])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24" style={{ color: '#9CA3AF' }}>
        <div
          className="h-6 w-6 animate-spin rounded-full border-2 border-t-transparent mr-3"
          style={{ borderColor: '#004899', borderTopColor: 'transparent' }}
        />
        Cargando aplicación...
      </div>
    )
  }

  if (!app) {
    return (
      <div className="text-center py-24" style={{ color: '#9CA3AF' }}>
        <p>Aplicación no encontrada.</p>
        <Button variant="outline" onClick={() => navigate('/applications')} className="mt-4">
          Volver a aplicaciones
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigate('/applications')}
            aria-label="Volver"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div
            className="h-10 w-10 rounded-lg flex items-center justify-center shrink-0"
            style={{ backgroundColor: '#004899' }}
          >
            <Monitor className="h-5 w-5 text-white" aria-hidden="true" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold" style={{ color: '#1A1A2E' }}>{app.name}</h1>
              {app.is_system && <Badge variant="system">Sistema</Badge>}
              <StatusBadge active={app.is_active} />
            </div>
            <p className="text-sm font-mono" style={{ color: '#6B7280' }}>{app.slug}</p>
          </div>
        </div>

        <Button
          variant="outline"
          size="sm"
          onClick={() => setShowEdit(true)}
          className="shrink-0 gap-1.5"
        >
          <Pencil className="h-3.5 w-3.5" />
          Editar
        </Button>
      </div>

      {/* Tabs */}
      <div
        className="bg-white rounded-xl overflow-hidden"
        style={{ border: '1px solid #E0E5EC' }}
      >
        {/* Tab bar */}
        <div
          className="flex overflow-x-auto"
          style={{ borderBottom: '1px solid #E0E5EC' }}
          role="tablist"
          aria-label="Secciones de la aplicación"
        >
          {tabs.map((tab) => (
            <button
              key={tab.id}
              role="tab"
              aria-selected={activeTab === tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={cn(
                'px-5 py-3.5 text-sm font-medium whitespace-nowrap transition-colors border-b-2 -mb-px',
                activeTab === tab.id
                  ? 'border-sodexo-blue text-sodexo-blue'
                  : 'border-transparent text-gray-500 hover:text-gray-800'
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        <div role="tabpanel">
          {activeTab === 'info' && <InfoTab app={app} />}
          {activeTab === 'roles' && <RolesTab appSlug={app.slug} />}
          {activeTab === 'permissions' && <PermissionsTab appSlug={app.slug} />}
          {activeTab === 'cost-centers' && <CostCentersTab appSlug={app.slug} />}
          {activeTab === 'users' && <UsersTab appSlug={app.slug} />}
        </div>
      </div>

      <ApplicationFormDialog
        open={showEdit}
        onOpenChange={setShowEdit}
        onSuccess={loadApp}
        app={app}
      />
    </div>
  )
}

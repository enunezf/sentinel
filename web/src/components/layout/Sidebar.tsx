import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard,
  Users,
  Monitor,
  Shield,
  Key,
  Building2,
  ClipboardList,
  Settings,
  X,
  ChevronLeft,
  ChevronRight,
  Timer,
} from 'lucide-react'
import { cn } from '@/lib/utils'

interface SidebarProps {
  open: boolean
  collapsed: boolean
  onClose: () => void
  onToggleCollapse: () => void
}

const mainNavItems = [
  { to: '/users', icon: Users, label: 'Usuarios' },
  { to: '/applications', icon: Monitor, label: 'Aplicaciones' },
  { to: '/roles', icon: Shield, label: 'Roles' },
  { to: '/permissions', icon: Key, label: 'Permisos' },
  { to: '/cost-centers', icon: Building2, label: 'Centros de Costo' },
]

export function Sidebar({ open, collapsed, onClose, onToggleCollapse }: SidebarProps) {
  const sidebarWidth = collapsed ? 'lg:w-16' : 'lg:w-60'

  return (
    <>
      {/* Mobile overlay */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={onClose}
          aria-hidden="true"
        />
      )}

      {/* Sidebar */}
      <aside
        className={cn(
          'fixed left-0 top-0 z-50 h-full flex flex-col transition-all duration-300',
          'lg:static lg:translate-x-0 lg:z-auto',
          'w-60',
          sidebarWidth,
          open ? 'translate-x-0' : '-translate-x-full'
        )}
        style={{ backgroundColor: '#1A2B4A' }}
        aria-label="Navegación principal"
      >
        {/* Logo area */}
        <div
          className="flex items-center justify-between px-4 border-b shrink-0"
          style={{ height: '64px', borderColor: 'rgba(255,255,255,0.1)' }}
        >
          {!collapsed && (
            <div className="flex items-center gap-2">
              <div className="h-8 w-8 rounded bg-sodexo-blue flex items-center justify-center shrink-0">
                <span className="text-white font-bold text-sm">S</span>
              </div>
              <div>
                <p className="text-white font-semibold text-sm leading-tight">Sentinel</p>
                <p className="text-white/50 text-xs leading-tight">Admin Panel</p>
              </div>
            </div>
          )}
          {collapsed && (
            <div className="flex justify-center w-full">
              <div className="h-8 w-8 rounded bg-sodexo-blue flex items-center justify-center">
                <span className="text-white font-bold text-sm">S</span>
              </div>
            </div>
          )}
          {/* Mobile close */}
          <button
            className="lg:hidden text-white/60 hover:text-white"
            onClick={onClose}
            aria-label="Cerrar menú"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Navigation */}
        <nav className="flex-1 overflow-y-auto py-3 space-y-0.5 px-2" aria-label="Menú de navegación">
          {/* Dashboard */}
          <NavItem to="/dashboard" icon={LayoutDashboard} label="Dashboard" collapsed={collapsed} onClose={onClose} />

          {/* Separator */}
          <div className="my-2 mx-1 border-t" style={{ borderColor: 'rgba(255,255,255,0.08)' }} />

          {/* Main items */}
          {mainNavItems.map(({ to, icon, label }) => (
            <NavItem key={to} to={to} icon={icon} label={label} collapsed={collapsed} onClose={onClose} />
          ))}

          {/* Separator */}
          <div className="my-2 mx-1 border-t" style={{ borderColor: 'rgba(255,255,255,0.08)' }} />

          {/* Auditoría */}
          <NavItem to="/audit" icon={ClipboardList} label="Auditoría" collapsed={collapsed} onClose={onClose} />
          <NavItem to="/temporal-assignments" icon={Timer} label="Asig. Temporales" collapsed={collapsed} onClose={onClose} />
        </nav>

        {/* Bottom: Configuración + collapse toggle */}
        <div className="shrink-0 px-2 pb-3 space-y-0.5 border-t" style={{ borderColor: 'rgba(255,255,255,0.08)', paddingTop: '8px' }}>
          <NavItem to="/configuracion" icon={Settings} label="Configuración" collapsed={collapsed} onClose={onClose} />

          {/* Desktop collapse toggle */}
          <button
            className={cn(
              'hidden lg:flex items-center w-full rounded px-3 py-2.5 text-sm transition-colors mt-1',
              'text-white/50 hover:text-white/80',
              collapsed ? 'justify-center' : 'gap-3'
            )}
            style={{ background: 'none' }}
            onClick={onToggleCollapse}
            aria-label={collapsed ? 'Expandir menú' : 'Colapsar menú'}
          >
            {collapsed ? (
              <ChevronRight className="h-4 w-4 shrink-0" />
            ) : (
              <>
                <ChevronLeft className="h-4 w-4 shrink-0" />
                <span>Colapsar menú</span>
              </>
            )}
          </button>
        </div>
      </aside>
    </>
  )
}

interface NavItemProps {
  to: string
  icon: React.ComponentType<{ className?: string }>
  label: string
  collapsed: boolean
  onClose: () => void
}

function NavItem({ to, icon: Icon, label, collapsed, onClose }: NavItemProps) {
  return (
    <NavLink
      to={to}
      title={collapsed ? label : undefined}
      className={({ isActive }) =>
        cn(
          'relative flex items-center rounded px-3 py-2.5 text-sm font-medium transition-colors',
          collapsed ? 'justify-center' : 'gap-3',
          isActive
            ? 'text-white border-l-[3px] border-white pl-[9px]'
            : 'text-white/80 border-l-[3px] border-transparent pl-[9px]'
        )
      }
      style={({ isActive }) => ({
        backgroundColor: isActive ? '#004899' : undefined,
      })}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLElement
        if (!el.style.backgroundColor || el.style.backgroundColor === '') {
          el.style.backgroundColor = 'rgba(255,255,255,0.08)'
        }
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLElement
        if (el.style.backgroundColor === 'rgba(255, 255, 255, 0.08)') {
          el.style.backgroundColor = ''
        }
      }}
      onClick={() => {
        if (window.innerWidth < 1024) onClose()
      }}
    >
      <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
      {!collapsed && <span className="truncate">{label}</span>}
    </NavLink>
  )
}

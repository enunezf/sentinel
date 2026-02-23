import { Menu, Settings, LogOut, User, KeyRound } from 'lucide-react'
import { useNavigate, Link } from 'react-router-dom'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useAuthStore } from '@/store/authStore'
import { toast } from '@/hooks/useToast'

interface HeaderProps {
  onMenuToggle: () => void
}

export function Header({ onMenuToggle }: HeaderProps) {
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    toast({ title: 'Sesión cerrada', variant: 'default' })
    navigate('/login', { replace: true })
  }

  const initials = user?.username
    ? user.username.slice(0, 2).toUpperCase()
    : 'AD'

  return (
    <header
      className="fixed top-0 right-0 left-0 z-30 flex items-center px-4 gap-3 bg-sodexo-blue shadow-sm"
      style={{ height: '64px' }}
      aria-label="Barra de navegación superior"
    >
      {/* Mobile menu toggle */}
      <button
        className="lg:hidden text-white/80 hover:text-white p-1.5 rounded transition-colors"
        onClick={onMenuToggle}
        aria-label="Abrir menú de navegación"
      >
        <Menu className="h-5 w-5" />
      </button>

      {/* Logo + título */}
      <div className="flex items-center gap-2 select-none">
        <div className="h-8 w-8 rounded bg-white/15 flex items-center justify-center">
          <span className="text-white font-bold text-sm leading-none">S</span>
        </div>
        <div className="hidden sm:block">
          <p className="text-white font-semibold text-sm leading-tight">Sodexo</p>
          <p className="text-white/70 text-xs leading-tight">Auth Admin</p>
        </div>
      </div>

      <div className="flex-1" />

      {/* Actions */}
      <div className="flex items-center gap-1">
        {/* Configuración */}
        <Link
          to="/configuracion"
          className="h-9 w-9 flex items-center justify-center rounded text-white/70 hover:text-white hover:bg-white/10 transition-colors"
          aria-label="Configuración del sistema"
        >
          <Settings className="h-4 w-4" />
        </Link>

        {/* User dropdown */}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-white/10 transition-colors text-white"
              aria-label="Menú de usuario"
            >
              <div className="h-7 w-7 rounded-full bg-white/20 flex items-center justify-center text-xs font-semibold shrink-0">
                {initials}
              </div>
              <span className="hidden sm:block text-sm font-medium max-w-[120px] truncate">
                {user?.username ?? 'Admin'}
              </span>
            </button>
          </DropdownMenu.Trigger>

          <DropdownMenu.Portal>
            <DropdownMenu.Content
              className="z-50 min-w-[180px] bg-white rounded-md border border-sodexo-gray-mid shadow-lg py-1 text-sm"
              sideOffset={8}
              align="end"
            >
              <div className="px-3 py-2 border-b border-sodexo-gray-mid">
                <p className="font-semibold text-sodexo-text-dark truncate">{user?.username}</p>
                <p className="text-sodexo-gray-text text-xs truncate">{user?.email ?? ''}</p>
              </div>

              <DropdownMenu.Item
                className="flex items-center gap-2 px-3 py-2 text-sodexo-text-dark hover:bg-sodexo-gray-light cursor-pointer outline-none"
                onSelect={() => navigate('/profile')}
              >
                <User className="h-4 w-4 text-sodexo-gray-text" />
                Mi perfil
              </DropdownMenu.Item>

              <DropdownMenu.Item
                className="flex items-center gap-2 px-3 py-2 text-sodexo-text-dark hover:bg-sodexo-gray-light cursor-pointer outline-none"
                onSelect={() => navigate('/profile?tab=password')}
              >
                <KeyRound className="h-4 w-4 text-sodexo-gray-text" />
                Cambiar contraseña
              </DropdownMenu.Item>

              <DropdownMenu.Separator className="my-1 border-t border-sodexo-gray-mid" />

              <DropdownMenu.Item
                className="flex items-center gap-2 px-3 py-2 text-sodexo-red hover:bg-red-50 cursor-pointer outline-none"
                onSelect={handleLogout}
              >
                <LogOut className="h-4 w-4" />
                Cerrar sesión
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </header>
  )
}

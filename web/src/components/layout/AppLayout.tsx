import { useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { Header } from './Header'

export function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)

  return (
    <div className="flex h-screen bg-sodexo-gray-light">
      <Sidebar
        open={sidebarOpen}
        collapsed={sidebarCollapsed}
        onClose={() => setSidebarOpen(false)}
        onToggleCollapse={() => setSidebarCollapsed((v) => !v)}
      />

      {/* Main content — shifts right to account for the sidebar width */}
      <div
        className="flex-1 flex flex-col min-w-0 transition-all duration-300"
        style={{ paddingLeft: sidebarCollapsed ? undefined : undefined }}
      >
        {/* On desktop, add left padding matching sidebar width */}
        <div
          className={
            sidebarCollapsed
              ? 'lg:pl-16 flex flex-col flex-1 min-w-0'
              : 'lg:pl-60 flex flex-col flex-1 min-w-0'
          }
        >
          <Header onMenuToggle={() => setSidebarOpen(true)} />

          <main
            className="flex-1 overflow-auto"
            style={{ paddingTop: '64px' }}
          >
            <div className="p-6 max-w-7xl mx-auto">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </div>
  )
}

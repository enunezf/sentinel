import { Routes, Route, Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuthStore } from '@/store/authStore'
import { AppLayout } from '@/components/layout/AppLayout'
import { LoginPage } from '@/pages/LoginPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { UsersPage } from '@/pages/users/UsersPage'
import { UserDetailPage } from '@/pages/users/UserDetailPage'
import { RolesPage } from '@/pages/roles/RolesPage'
import { RoleDetailPage } from '@/pages/roles/RoleDetailPage'
import { PermissionsPage } from '@/pages/permissions/PermissionsPage'
import { CostCentersPage } from '@/pages/costCenters/CostCentersPage'
import { AuditLogsPage } from '@/pages/audit/AuditLogsPage'

// Páginas en construcción — se implementan en fases posteriores
import { ApplicationsPage } from '@/pages/applications/ApplicationsPage'
import { ApplicationDetailPage } from '@/pages/applications/ApplicationDetailPage'
import { ProfilePage } from '@/pages/ProfilePage'
import { SystemConfigPage } from '@/pages/SystemConfigPage'
import { TemporalAssignmentsPage } from '@/pages/TemporalAssignmentsPage'

function PrivateRoute({ children }: { children: ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const accessToken = localStorage.getItem('sentinel_access_token')

  if (!isAuthenticated && !accessToken) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <PrivateRoute>
            <AppLayout />
          </PrivateRoute>
        }
      >
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/users/:id" element={<UserDetailPage />} />
        <Route path="/applications" element={<ApplicationsPage />} />
        <Route path="/applications/:id" element={<ApplicationDetailPage />} />
        <Route path="/roles" element={<RolesPage />} />
        <Route path="/roles/:id" element={<RoleDetailPage />} />
        <Route path="/permissions" element={<PermissionsPage />} />
        <Route path="/cost-centers" element={<CostCentersPage />} />
        <Route path="/audit" element={<AuditLogsPage />} />
        <Route path="/profile" element={<ProfilePage />} />
        <Route path="/configuracion" element={<SystemConfigPage />} />
        <Route path="/temporal-assignments" element={<TemporalAssignmentsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  )
}

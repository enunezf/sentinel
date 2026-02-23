export interface User {
  id: string
  username: string
  email: string
  is_active: boolean
  must_change_pwd: boolean
  last_login_at: string | null
  failed_attempts: number
  locked_until: string | null
  created_at: string
  updated_at?: string
  roles?: UserRole[]
  permissions?: UserPermission[]
  cost_centers?: UserCostCenter[]
}

export interface UserRole {
  id: string
  name: string
  application: string
  valid_from: string
  valid_until: string | null
  is_active: boolean
}

export interface UserPermission {
  id: string
  code: string
  application: string
  valid_from: string
  valid_until: string | null
}

export interface UserCostCenter {
  id: string
  code: string
  name: string
  application: string
}

export interface Application {
  id: string
  name: string
  slug: string
  secret_key?: string
  is_active: boolean
  is_system: boolean
  created_at: string
  updated_at?: string
}

export interface Role {
  id: string
  name: string
  description: string | null
  is_system: boolean
  is_active: boolean
  permissions_count?: number
  permissions?: Permission[]
  users_count?: number
  created_at: string
  updated_at?: string
}

export interface Permission {
  id: string
  code: string
  description: string | null
  scope_type: 'global' | 'module' | 'resource' | 'action'
  created_at: string
}

export interface CostCenter {
  id: string
  code: string
  name: string
  is_active: boolean
  application_id?: string
  created_at: string
}

export interface AuditLog {
  id: string
  event_type: string
  application_id: string | null
  user_id: string | null
  actor_id: string | null
  resource_type: string | null
  resource_id: string | null
  old_value: Record<string, unknown> | null
  new_value: Record<string, unknown> | null
  ip_address: string | null
  user_agent: string | null
  success: boolean
  error_message: string | null
  created_at: string
}

export interface PaginatedResponse<T> {
  data: T[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface LoginResponse {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
  user: {
    id: string
    username: string
    email: string
    must_change_password: boolean
  }
}

export interface RefreshResponse {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
}

export interface ApiError {
  error: {
    code: string
    message: string
    details: Record<string, unknown> | null
  }
}

export interface RoleAssignment {
  id: string
  user_id: string
  role_id: string
  role_name: string
  valid_from: string
  valid_until: string | null
  granted_by: string
}

export interface ResetPasswordResponse {
  temporary_password: string
}

export type EventType =
  | 'AUTH_LOGIN_SUCCESS'
  | 'AUTH_LOGIN_FAILED'
  | 'AUTH_ACCOUNT_LOCKED'
  | 'AUTH_LOGOUT'
  | 'AUTH_TOKEN_REFRESHED'
  | 'AUTH_PASSWORD_CHANGED'
  | 'AUTH_PASSWORD_RESET'
  | 'USER_CREATED'
  | 'USER_UPDATED'
  | 'USER_DEACTIVATED'
  | 'USER_UNLOCKED'
  | 'USER_ROLE_ASSIGNED'
  | 'USER_ROLE_REVOKED'
  | 'USER_PERMISSION_ASSIGNED'
  | 'USER_PERMISSION_REVOKED'
  | 'USER_COST_CENTER_ASSIGNED'
  | 'ROLE_CREATED'
  | 'ROLE_UPDATED'
  | 'ROLE_DELETED'
  | 'ROLE_PERMISSION_ASSIGNED'
  | 'ROLE_PERMISSION_REVOKED'
  | 'PERMISSION_CREATED'
  | 'PERMISSION_DELETED'

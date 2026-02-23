import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Eye, EyeOff, CheckCircle2, XCircle, AlertTriangle } from 'lucide-react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuthStore } from '@/store/authStore'
import { getErrorMessage } from '@/lib/utils'
import { authApi } from '@/api/auth'

// ─── Schemas ───────────────────────────────────────────────────────────────

const loginSchema = z.object({
  username: z.string().min(1, 'El usuario es requerido').max(100),
  password: z.string().min(1, 'La contraseña es requerida'),
})

type LoginFormData = z.infer<typeof loginSchema>

const changePasswordSchema = z
  .object({
    current_password: z.string().min(1, 'La contraseña actual es requerida'),
    new_password: z
      .string()
      .min(10, 'Mínimo 10 caracteres')
      .regex(/[A-Z]/, 'Debe contener al menos una mayúscula')
      .regex(/[0-9]/, 'Debe contener al menos un número')
      .regex(/[^A-Za-z0-9]/, 'Debe contener al menos un símbolo'),
    confirm_password: z.string().min(1, 'Confirme la nueva contraseña'),
  })
  .refine((d) => d.new_password === d.confirm_password, {
    message: 'Las contraseñas no coinciden',
    path: ['confirm_password'],
  })

type ChangePasswordFormData = z.infer<typeof changePasswordSchema>

// ─── Error alert variant ────────────────────────────────────────────────────

type AlertVariant = 'error' | 'warning' | 'info'

function getErrorVariant(code: string): AlertVariant {
  if (code === 'ACCOUNT_LOCKED') return 'warning'
  if (code === 'ACCOUNT_INACTIVE') return 'info'
  return 'error'
}

const alertStyles: Record<AlertVariant, string> = {
  error: 'bg-red-50 border-red-200 text-red-800',
  warning: 'bg-amber-50 border-amber-200 text-amber-800',
  info: 'bg-gray-100 border-gray-300 text-gray-700',
}

const alertIconStyles: Record<AlertVariant, string> = {
  error: 'text-red-500',
  warning: 'text-amber-500',
  info: 'text-gray-500',
}

// ─── Password policy indicators ────────────────────────────────────────────

interface PolicyRule {
  label: string
  test: (v: string) => boolean
}

const passwordPolicies: PolicyRule[] = [
  { label: 'Mínimo 10 caracteres', test: (v) => v.length >= 10 },
  { label: 'Una letra mayúscula', test: (v) => /[A-Z]/.test(v) },
  { label: 'Un número', test: (v) => /[0-9]/.test(v) },
  { label: 'Un símbolo especial', test: (v) => /[^A-Za-z0-9]/.test(v) },
]

function PolicyIndicator({ value }: { value: string }) {
  return (
    <ul className="mt-2 space-y-1">
      {passwordPolicies.map((rule) => {
        const ok = rule.test(value)
        return (
          <li key={rule.label} className={`flex items-center gap-1.5 text-xs ${ok ? 'text-green-600' : 'text-gray-500'}`}>
            {ok ? (
              <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-green-500" aria-hidden="true" />
            ) : (
              <XCircle className="h-3.5 w-3.5 shrink-0 text-gray-400" aria-hidden="true" />
            )}
            {rule.label}
          </li>
        )
      })}
    </ul>
  )
}

// ─── Mandatory password change modal ───────────────────────────────────────

interface ChangePasswordModalProps {
  onSuccess: () => void
}

function ChangePasswordModal({ onSuccess }: ChangePasswordModalProps) {
  const [showCurrent, setShowCurrent] = useState(false)
  const [showNew, setShowNew] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    watch,
    formState: { errors },
  } = useForm<ChangePasswordFormData>({
    resolver: zodResolver(changePasswordSchema),
    defaultValues: { current_password: '', new_password: '', confirm_password: '' },
  })

  const newPasswordValue = watch('new_password') ?? ''

  const onSubmit = async (data: ChangePasswordFormData) => {
    setServerError(null)
    setIsSubmitting(true)
    try {
      await authApi.changePassword({
        current_password: data.current_password,
        new_password: data.new_password,
      })
      onSuccess()
    } catch (err) {
      const code = err instanceof Error ? err.message : 'INTERNAL_ERROR'
      setServerError(getErrorMessage(code))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <DialogPrimitive.Root open modal>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm" />
        <DialogPrimitive.Content
          className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg bg-white p-6 shadow-xl"
          onPointerDownOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
          aria-describedby="change-pwd-desc"
        >
          {/* Header */}
          <div className="mb-5">
            <div className="flex items-center gap-3 mb-2">
              <div
                className="h-10 w-10 rounded-full flex items-center justify-center shrink-0"
                style={{ backgroundColor: '#FEF3C7' }}
              >
                <AlertTriangle className="h-5 w-5 text-amber-600" aria-hidden="true" />
              </div>
              <div>
                <DialogPrimitive.Title className="text-base font-semibold text-gray-900">
                  Cambio de contraseña requerido
                </DialogPrimitive.Title>
                <p id="change-pwd-desc" className="text-xs text-gray-500 mt-0.5">
                  Debe cambiar su contraseña antes de continuar.
                </p>
              </div>
            </div>
          </div>

          {serverError && (
            <div className="mb-4 rounded-md bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-800" role="alert">
              {serverError}
            </div>
          )}

          <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
            {/* Current password */}
            <div className="space-y-1.5">
              <Label htmlFor="current_password">Contraseña actual</Label>
              <div className="relative">
                <Input
                  id="current_password"
                  type={showCurrent ? 'text' : 'password'}
                  autoComplete="current-password"
                  placeholder="••••••••••"
                  aria-invalid={!!errors.current_password}
                  {...register('current_password')}
                />
                <button
                  type="button"
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
                  onClick={() => setShowCurrent((v) => !v)}
                  aria-label={showCurrent ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                >
                  {showCurrent ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {errors.current_password && (
                <p className="text-xs text-red-600" role="alert">{errors.current_password.message}</p>
              )}
            </div>

            {/* New password */}
            <div className="space-y-1.5">
              <Label htmlFor="new_password">Nueva contraseña</Label>
              <div className="relative">
                <Input
                  id="new_password"
                  type={showNew ? 'text' : 'password'}
                  autoComplete="new-password"
                  placeholder="••••••••••"
                  aria-invalid={!!errors.new_password}
                  {...register('new_password')}
                />
                <button
                  type="button"
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
                  onClick={() => setShowNew((v) => !v)}
                  aria-label={showNew ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                >
                  {showNew ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {errors.new_password && (
                <p className="text-xs text-red-600" role="alert">{errors.new_password.message}</p>
              )}
              <PolicyIndicator value={newPasswordValue} />
            </div>

            {/* Confirm password */}
            <div className="space-y-1.5">
              <Label htmlFor="confirm_password">Confirmar nueva contraseña</Label>
              <div className="relative">
                <Input
                  id="confirm_password"
                  type={showConfirm ? 'text' : 'password'}
                  autoComplete="new-password"
                  placeholder="••••••••••"
                  aria-invalid={!!errors.confirm_password}
                  {...register('confirm_password')}
                />
                <button
                  type="button"
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
                  onClick={() => setShowConfirm((v) => !v)}
                  aria-label={showConfirm ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                >
                  {showConfirm ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {errors.confirm_password && (
                <p className="text-xs text-red-600" role="alert">{errors.confirm_password.message}</p>
              )}
            </div>

            <Button type="submit" className="w-full mt-2" disabled={isSubmitting}>
              {isSubmitting ? (
                <span className="flex items-center gap-2">
                  <span className="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
                  Cambiando contraseña...
                </span>
              ) : (
                'Cambiar contraseña'
              )}
            </Button>
          </form>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

// ─── Main Login Page ────────────────────────────────────────────────────────

export function LoginPage() {
  const navigate = useNavigate()
  const { login, isAuthenticated, isLoading } = useAuthStore()
  const [showPassword, setShowPassword] = useState(false)
  const [errorCode, setErrorCode] = useState<string | null>(null)
  const [showChangeModal, setShowChangeModal] = useState(false)

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  })

  useEffect(() => {
    if (isAuthenticated) {
      navigate('/dashboard', { replace: true })
    }
  }, [isAuthenticated, navigate])

  const onSubmit = async (data: LoginFormData) => {
    setErrorCode(null)
    try {
      await login({ ...data, client_type: 'web' })
      const user = useAuthStore.getState().user
      if (user?.must_change_password) {
        setShowChangeModal(true)
      } else {
        navigate('/dashboard', { replace: true })
      }
    } catch (err) {
      const code = err instanceof Error ? err.message : 'INTERNAL_ERROR'
      setErrorCode(code)
    }
  }

  const handlePasswordChanged = () => {
    setShowChangeModal(false)
    navigate('/dashboard', { replace: true })
  }

  const variant = errorCode ? getErrorVariant(errorCode) : 'error'

  return (
    <>
      {showChangeModal && <ChangePasswordModal onSuccess={handlePasswordChanged} />}

      <div className="min-h-screen flex items-center justify-center px-4" style={{ backgroundColor: '#F4F6F9' }}>
        <div className="w-full max-w-md">
          {/* Logo + title */}
          <div className="text-center mb-8">
            <div
              className="inline-flex items-center justify-center h-16 w-16 rounded-full mb-4"
              style={{ backgroundColor: '#004899' }}
            >
              <span className="text-white font-bold text-2xl select-none">S</span>
            </div>
            <h1 className="text-2xl font-bold" style={{ color: '#1A1A2E' }}>
              Sodexo
            </h1>
            <p className="text-sm mt-1" style={{ color: '#6B7280' }}>
              Auth Admin Panel
            </p>
          </div>

          {/* Card */}
          <div className="bg-white rounded-xl border shadow-sm p-8" style={{ borderColor: '#E0E5EC' }}>
            <h2 className="text-lg font-semibold mb-6" style={{ color: '#1A1A2E' }}>
              Iniciar sesión
            </h2>

            <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
              {/* Error alert */}
              {errorCode && (
                <div
                  className={`rounded-md border px-4 py-3 text-sm flex items-start gap-2 ${alertStyles[variant]}`}
                  role="alert"
                >
                  <AlertTriangle className={`h-4 w-4 mt-0.5 shrink-0 ${alertIconStyles[variant]}`} aria-hidden="true" />
                  <span>{getErrorMessage(errorCode)}</span>
                </div>
              )}

              {/* Username */}
              <div className="space-y-1.5">
                <Label htmlFor="username">Usuario</Label>
                <Input
                  id="username"
                  type="text"
                  autoComplete="username"
                  placeholder="jperez"
                  aria-invalid={!!errors.username}
                  aria-describedby={errors.username ? 'username-error' : undefined}
                  {...register('username')}
                />
                {errors.username && (
                  <p id="username-error" className="text-xs text-red-600" role="alert">
                    {errors.username.message}
                  </p>
                )}
              </div>

              {/* Password */}
              <div className="space-y-1.5">
                <Label htmlFor="password">Contraseña</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="current-password"
                    placeholder="••••••••••"
                    aria-invalid={!!errors.password}
                    aria-describedby={errors.password ? 'password-error' : undefined}
                    {...register('password')}
                  />
                  <button
                    type="button"
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
                    onClick={() => setShowPassword((v) => !v)}
                    aria-label={showPassword ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                {errors.password && (
                  <p id="password-error" className="text-xs text-red-600" role="alert">
                    {errors.password.message}
                  </p>
                )}
              </div>

              <Button type="submit" className="w-full mt-6" disabled={isLoading}>
                {isLoading ? (
                  <span className="flex items-center gap-2">
                    <span className="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
                    Iniciando sesión...
                  </span>
                ) : (
                  'Iniciar sesión'
                )}
              </Button>
            </form>
          </div>

          {/* Footer */}
          <p className="text-center text-xs mt-6" style={{ color: '#9CA3AF' }}>
            Sodexo Chile © {new Date().getFullYear()} — Uso interno
          </p>
        </div>
      </div>
    </>
  )
}

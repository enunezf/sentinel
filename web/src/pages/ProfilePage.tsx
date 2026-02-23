import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { User, Mail, Key, CheckCircle2, XCircle, Shield } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { authApi } from '@/api/auth'
import type { ApiError } from '@/types'
import { PageHeader } from '@/components/shared/PageHeader'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/hooks/useToast'
import type { AxiosError } from 'axios'

// ─── Password policy ─────────────────────────────────────────────────────────

const POLICY = [
  { id: 'len', label: 'Mínimo 10 caracteres', test: (v: string) => v.length >= 10 },
  { id: 'upper', label: 'Una letra mayúscula', test: (v: string) => /[A-Z]/.test(v) },
  { id: 'num', label: 'Un número', test: (v: string) => /[0-9]/.test(v) },
  { id: 'sym', label: 'Un símbolo (!@#$...)', test: (v: string) => /[^A-Za-z0-9]/.test(v) },
]

function PolicyRow({ ok, label }: { ok: boolean; label: string }) {
  return (
    <div className="flex items-center gap-1.5 text-xs">
      {ok ? (
        <CheckCircle2 className="h-3.5 w-3.5 shrink-0" style={{ color: '#16A34A' }} />
      ) : (
        <XCircle className="h-3.5 w-3.5 shrink-0" style={{ color: '#9CA3AF' }} />
      )}
      <span style={{ color: ok ? '#16A34A' : '#9CA3AF' }}>{label}</span>
    </div>
  )
}

// ─── Schema ───────────────────────────────────────────────────────────────────

const schema = z
  .object({
    current_password: z.string().min(1, 'Requerido'),
    new_password: z
      .string()
      .min(10, 'Mínimo 10 caracteres')
      .regex(/[A-Z]/, 'Debe incluir una mayúscula')
      .regex(/[0-9]/, 'Debe incluir un número')
      .regex(/[^A-Za-z0-9]/, 'Debe incluir un símbolo'),
    confirm: z.string().min(1, 'Requerido'),
  })
  .refine((d) => d.new_password === d.confirm, {
    message: 'Las contraseñas no coinciden',
    path: ['confirm'],
  })

type FormData = z.infer<typeof schema>

// ─── Page ─────────────────────────────────────────────────────────────────────

export function ProfilePage() {
  const user = useAuthStore((s) => s.user)
  const [newPwd, setNewPwd] = useState('')
  const [showSuccess, setShowSuccess] = useState(false)

  const {
    register,
    handleSubmit,
    reset,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<FormData>({ resolver: zodResolver(schema) })

  const watchedNew = watch('new_password') ?? newPwd

  const onSubmit = async (data: FormData) => {
    try {
      await authApi.changePassword({
        current_password: data.current_password,
        new_password: data.new_password,
      })
      toast({ title: 'Contraseña actualizada', description: 'Tu contraseña fue cambiada exitosamente.' })
      reset()
      setNewPwd('')
      setShowSuccess(true)
      setTimeout(() => setShowSuccess(false), 4000)
    } catch (err) {
      const axiosErr = err as AxiosError<ApiError>
      const code = axiosErr.response?.data?.error?.code ?? 'INTERNAL_ERROR'
      const msgs: Record<string, string> = {
        INVALID_CREDENTIALS: 'La contraseña actual es incorrecta.',
        PASSWORD_REUSED: 'Esta contraseña ya fue usada anteriormente (últimas 5).',
      }
      toast({
        title: 'Error al cambiar contraseña',
        description: msgs[code] ?? `Error: ${code}`,
        variant: 'destructive',
      })
    }
  }

  const initials = user?.username?.slice(0, 2).toUpperCase() ?? '?'

  return (
    <div>
      <PageHeader title="Mi Perfil" description="Información de tu cuenta de administrador" />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* ── Info card ── */}
        <div
          className="rounded-xl p-6 space-y-5"
          style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
        >
          {/* Avatar */}
          <div className="flex flex-col items-center text-center gap-3">
            <div
              className="h-20 w-20 rounded-full flex items-center justify-center text-2xl font-bold text-white"
              style={{ backgroundColor: '#004899' }}
            >
              {initials}
            </div>
            <div>
              <h2 className="font-semibold text-lg" style={{ color: '#1A1A2E' }}>
                {user?.username ?? '—'}
              </h2>
              <p className="text-sm" style={{ color: '#6B7280' }}>Administrador</p>
            </div>
            <span
              className="text-xs px-2.5 py-1 rounded-full font-medium"
              style={{ backgroundColor: '#EFF6FF', color: '#004899' }}
            >
              <Shield className="h-3 w-3 inline mr-1 -mt-0.5" />
              Admin
            </span>
          </div>

          <hr style={{ borderColor: '#E0E5EC' }} />

          {/* Details */}
          <div className="space-y-3 text-sm">
            <div className="flex items-start gap-3">
              <User className="h-4 w-4 mt-0.5 shrink-0" style={{ color: '#9CA3AF' }} />
              <div>
                <p className="text-xs font-medium" style={{ color: '#6B7280' }}>Usuario</p>
                <p style={{ color: '#1A1A2E' }}>{user?.username ?? '—'}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <Mail className="h-4 w-4 mt-0.5 shrink-0" style={{ color: '#9CA3AF' }} />
              <div>
                <p className="text-xs font-medium" style={{ color: '#6B7280' }}>Correo electrónico</p>
                <p style={{ color: '#1A1A2E' }}>{user?.email ?? '—'}</p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <Key className="h-4 w-4 mt-0.5 shrink-0" style={{ color: '#9CA3AF' }} />
              <div>
                <p className="text-xs font-medium" style={{ color: '#6B7280' }}>ID de usuario</p>
                <p className="font-mono text-xs break-all" style={{ color: '#374151' }}>
                  {user?.id ?? '—'}
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* ── Change password card ── */}
        <div
          className="lg:col-span-2 rounded-xl p-6"
          style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
        >
          <div className="flex items-center gap-2 mb-5">
            <div
              className="h-8 w-8 rounded-lg flex items-center justify-center"
              style={{ backgroundColor: '#EFF6FF' }}
            >
              <Key className="h-4 w-4" style={{ color: '#004899' }} />
            </div>
            <div>
              <h3 className="font-semibold text-sm" style={{ color: '#1A1A2E' }}>
                Cambiar contraseña
              </h3>
              <p className="text-xs" style={{ color: '#6B7280' }}>
                Actualiza tu contraseña de acceso al panel
              </p>
            </div>
          </div>

          {showSuccess && (
            <div
              className="flex items-center gap-2 text-sm rounded-lg p-3 mb-4"
              style={{ backgroundColor: '#F0FDF4', border: '1px solid #BBF7D0', color: '#15803D' }}
            >
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              Contraseña actualizada correctamente.
            </div>
          )}

          <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="current-pwd">Contraseña actual *</Label>
              <Input
                id="current-pwd"
                type="password"
                autoComplete="current-password"
                aria-invalid={!!errors.current_password}
                {...register('current_password')}
              />
              {errors.current_password && (
                <p className="text-xs text-red-600" role="alert">
                  {errors.current_password.message}
                </p>
              )}
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="new-pwd">Nueva contraseña *</Label>
              <Input
                id="new-pwd"
                type="password"
                autoComplete="new-password"
                aria-invalid={!!errors.new_password}
                {...register('new_password', {
                  onChange: (e: React.ChangeEvent<HTMLInputElement>) => setNewPwd(e.target.value),
                })}
              />
              {errors.new_password && (
                <p className="text-xs text-red-600" role="alert">
                  {errors.new_password.message}
                </p>
              )}
              {/* Policy indicators */}
              {(watchedNew || newPwd) && (
                <div className="grid grid-cols-2 gap-1 mt-2 p-2.5 rounded-lg" style={{ backgroundColor: '#F4F6F9' }}>
                  {POLICY.map((p) => (
                    <PolicyRow key={p.id} ok={p.test(watchedNew || newPwd)} label={p.label} />
                  ))}
                </div>
              )}
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="confirm-pwd">Confirmar nueva contraseña *</Label>
              <Input
                id="confirm-pwd"
                type="password"
                autoComplete="new-password"
                aria-invalid={!!errors.confirm}
                {...register('confirm')}
              />
              {errors.confirm && (
                <p className="text-xs text-red-600" role="alert">
                  {errors.confirm.message}
                </p>
              )}
            </div>

            <div className="flex justify-end pt-2">
              <Button type="submit" disabled={isSubmitting}>
                {isSubmitting ? 'Guardando...' : 'Cambiar contraseña'}
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}

import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Eye, EyeOff, RefreshCw, Copy, Check } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ConfirmDialog } from '@/components/shared/ConfirmDialog'
import { applicationsApi } from '@/api/applications'
import { toast } from '@/hooks/useToast'
import type { Application, ApiError } from '@/types'
import type { AxiosError } from 'axios'

// ─── Schema ────────────────────────────────────────────────────────────────

const slugRegex = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

const createSchema = z.object({
  name: z.string().min(1, 'El nombre es requerido').max(100),
  slug: z
    .string()
    .min(1, 'El slug es requerido')
    .max(50)
    .regex(slugRegex, 'Solo minúsculas, números y guiones (ej: mi-app)'),
})

const editSchema = z.object({
  name: z.string().min(1, 'El nombre es requerido').max(100),
  is_active: z.boolean(),
})

type CreateFormData = z.infer<typeof createSchema>
type EditFormData = z.infer<typeof editSchema>

// ─── Slug auto-generation ──────────────────────────────────────────────────

function nameToSlug(name: string): string {
  return name
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '') // remove diacritics
    .replace(/[^a-z0-9\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .slice(0, 50)
}

// ─── Create form ───────────────────────────────────────────────────────────

interface CreateFormProps {
  onSuccess: () => void
  onCancel: () => void
}

function CreateForm({ onSuccess, onCancel }: CreateFormProps) {
  const {
    register,
    handleSubmit,
    setValue,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<CreateFormData>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: '', slug: '' },
  })

  const nameValue = watch('name')

  // Auto-generate slug from name
  useEffect(() => {
    if (nameValue) {
      setValue('slug', nameToSlug(nameValue), { shouldValidate: false })
    }
  }, [nameValue, setValue])

  const onSubmit = async (data: CreateFormData) => {
    try {
      await applicationsApi.create(data)
      toast({ title: 'Aplicación creada', description: `"${data.name}" creada exitosamente.` })
      onSuccess()
    } catch (err) {
      const axiosErr = err as AxiosError<ApiError>
      const msg = axiosErr.response?.data?.error?.message ?? 'No se pudo crear la aplicación.'
      toast({ title: 'Error', description: msg, variant: 'destructive' })
    }
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="app-name">Nombre *</Label>
        <Input
          id="app-name"
          placeholder="Mi Aplicación"
          aria-invalid={!!errors.name}
          {...register('name')}
        />
        {errors.name && <p className="text-xs text-red-600" role="alert">{errors.name.message}</p>}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="app-slug">
          Slug *
          <span className="text-xs font-normal text-gray-400 ml-2">Auto-generado del nombre</span>
        </Label>
        <Input
          id="app-slug"
          placeholder="mi-aplicacion"
          className="font-mono"
          aria-invalid={!!errors.slug}
          {...register('slug')}
        />
        {errors.slug ? (
          <p className="text-xs text-red-600" role="alert">{errors.slug.message}</p>
        ) : (
          <p className="text-xs text-gray-400">
            Solo minúsculas, números y guiones. Ejemplo: <code className="font-mono">mi-app</code>
          </p>
        )}
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel} disabled={isSubmitting}>
          Cancelar
        </Button>
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting ? 'Creando...' : 'Crear aplicación'}
        </Button>
      </DialogFooter>
    </form>
  )
}

// ─── Edit form ─────────────────────────────────────────────────────────────

interface EditFormProps {
  app: Application
  onSuccess: () => void
  onCancel: () => void
}

function EditForm({ app, onSuccess, onCancel }: EditFormProps) {
  const [showKey, setShowKey] = useState(false)
  const [secretKey, setSecretKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [confirmRotate, setConfirmRotate] = useState(false)
  const [isRotating, setIsRotating] = useState(false)

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<EditFormData>({
    resolver: zodResolver(editSchema),
    defaultValues: { name: app.name, is_active: app.is_active },
  })

  const onSubmit = async (data: EditFormData) => {
    try {
      await applicationsApi.update(app.id, data)
      toast({ title: 'Aplicación actualizada', description: `"${data.name}" actualizada.` })
      onSuccess()
    } catch (err) {
      const axiosErr = err as AxiosError<ApiError>
      const msg = axiosErr.response?.data?.error?.message ?? 'No se pudo actualizar la aplicación.'
      toast({ title: 'Error', description: msg, variant: 'destructive' })
    }
  }

  const handleRotate = async () => {
    setIsRotating(true)
    try {
      const res = await applicationsApi.rotateKey(app.id)
      setSecretKey(res.secret_key)
      setShowKey(true)
      setConfirmRotate(false)
      toast({ title: 'Secret key rotada', description: 'Guarde la nueva clave en un lugar seguro.' })
    } catch {
      toast({ title: 'Error al rotar la clave', variant: 'destructive' })
    } finally {
      setIsRotating(false)
    }
  }

  const handleCopy = () => {
    if (!secretKey) return
    void navigator.clipboard.writeText(secretKey)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const displayKey = secretKey ?? '••••••••••••••••••••••••••••••••••••••••••'

  return (
    <>
      <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
        {/* Slug — read-only */}
        <div className="space-y-1.5">
          <Label>Slug</Label>
          <div
            className="flex items-center h-9 rounded-md px-3 text-sm font-mono border"
            style={{ backgroundColor: '#F9FAFB', borderColor: '#E0E5EC', color: '#6B7280' }}
          >
            {app.slug}
          </div>
          <p className="text-xs text-gray-400">El slug no puede modificarse después de la creación.</p>
        </div>

        {/* Name */}
        <div className="space-y-1.5">
          <Label htmlFor="edit-name">Nombre *</Label>
          <Input
            id="edit-name"
            aria-invalid={!!errors.name}
            disabled={app.is_system}
            {...register('name')}
          />
          {errors.name && <p className="text-xs text-red-600" role="alert">{errors.name.message}</p>}
          {app.is_system && (
            <p className="text-xs text-amber-600">Las aplicaciones del sistema no pueden cambiar nombre.</p>
          )}
        </div>

        {/* Active state */}
        <div className="flex items-center gap-3">
          <input
            id="edit-active"
            type="checkbox"
            className="h-4 w-4 rounded border-gray-300 accent-sodexo-blue"
            disabled={app.is_system}
            {...register('is_active')}
          />
          <Label htmlFor="edit-active">Aplicación activa</Label>
        </div>

        {/* Secret key section */}
        <div className="rounded-lg p-4 space-y-3" style={{ backgroundColor: '#F9FAFB', border: '1px solid #E0E5EC' }}>
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium" style={{ color: '#1A1A2E' }}>
              Secret Key
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={app.is_system}
              onClick={() => setConfirmRotate(true)}
              className="gap-1.5 text-xs"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              Rotar clave
            </Button>
          </div>

          <div className="flex items-center gap-2">
            <div
              className="flex-1 font-mono text-sm rounded-md px-3 py-2 border overflow-hidden text-ellipsis whitespace-nowrap"
              style={{ backgroundColor: '#FFFFFF', borderColor: '#E0E5EC', color: secretKey ? '#1A1A2E' : '#9CA3AF' }}
            >
              {showKey && secretKey ? secretKey : displayKey}
            </div>
            {secretKey && (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => setShowKey((v) => !v)}
                  aria-label={showKey ? 'Ocultar clave' : 'Mostrar clave'}
                  title={showKey ? 'Ocultar clave' : 'Mostrar clave'}
                >
                  {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={handleCopy}
                  aria-label="Copiar clave"
                  title="Copiar clave"
                >
                  {copied ? (
                    <Check className="h-4 w-4 text-green-600" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </Button>
              </>
            )}
          </div>

          {secretKey && (
            <p className="text-xs text-amber-700 flex items-center gap-1.5">
              ⚠ Guarde esta clave ahora. No podrá verla de nuevo.
            </p>
          )}
          {app.is_system && (
            <p className="text-xs text-gray-400">La clave de aplicaciones del sistema no puede rotarse.</p>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onCancel} disabled={isSubmitting}>
            Cancelar
          </Button>
          <Button type="submit" disabled={isSubmitting || app.is_system}>
            {isSubmitting ? 'Guardando...' : 'Guardar cambios'}
          </Button>
        </DialogFooter>
      </form>

      <ConfirmDialog
        open={confirmRotate}
        onOpenChange={(open) => { if (!open) setConfirmRotate(false) }}
        title="Rotar secret key"
        description={`¿Confirma rotar la secret key de "${app.name}"? La clave anterior dejará de funcionar inmediatamente. Todas las integraciones deberán actualizarse.`}
        confirmLabel="Rotar clave"
        variant="destructive"
        onConfirm={handleRotate}
        isLoading={isRotating}
      />
    </>
  )
}

// ─── Public component ───────────────────────────────────────────────────────

interface ApplicationFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
  app?: Application | null
}

export function ApplicationFormDialog({
  open,
  onOpenChange,
  onSuccess,
  app,
}: ApplicationFormDialogProps) {
  const isEditing = !!app

  const handleSuccess = () => {
    onOpenChange(false)
    onSuccess()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Editar aplicación' : 'Nueva aplicación'}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Modifique los datos de la aplicación.'
              : 'Complete los datos para registrar una nueva aplicación.'}
          </DialogDescription>
        </DialogHeader>

        {isEditing ? (
          <EditForm
            app={app}
            onSuccess={handleSuccess}
            onCancel={() => onOpenChange(false)}
          />
        ) : (
          <CreateForm
            onSuccess={handleSuccess}
            onCancel={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

import { useEffect, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { permissionsApi } from '@/api/permissions'
import { usersApi } from '@/api/users'
import type { Permission } from '@/types'
import { toast } from '@/hooks/useToast'

interface AsignarPermisoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  username: string
  onSuccess: () => void
}

export function AsignarPermisoModal({
  open,
  onOpenChange,
  userId,
  username,
  onSuccess,
}: AsignarPermisoModalProps) {
  const [allPermissions, setAllPermissions] = useState<Permission[]>([])
  const [search, setSearch] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [isTemporal, setIsTemporal] = useState(false)
  const [validFrom, setValidFrom] = useState('')
  const [validUntil, setValidUntil] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (open) {
      setSearch('')
      setSelectedId('')
      setIsTemporal(false)
      setValidFrom('')
      setValidUntil('')
      void permissionsApi.list({ page_size: 100 }).then((r) => setAllPermissions(r.data))
    }
  }, [open])

  const filtered = search
    ? allPermissions.filter((p) => p.code.toLowerCase().includes(search.toLowerCase()))
    : allPermissions

  const handleSubmit = async () => {
    if (!selectedId) return
    setIsSubmitting(true)
    try {
      await usersApi.assignPermission(userId, {
        permission_id: selectedId,
        valid_from: isTemporal && validFrom ? validFrom : undefined,
        valid_until: isTemporal && validUntil ? validUntil : null,
      })
      const code = allPermissions.find((p) => p.id === selectedId)?.code ?? ''
      toast({ title: 'Permiso asignado', description: `"${code}" asignado a @${username}.` })
      onOpenChange(false)
      onSuccess()
    } catch {
      toast({ title: 'Error al asignar permiso', variant: 'destructive' })
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Asignar permiso</DialogTitle>
          <DialogDescription>
            Asignar un permiso especial a <strong>@{username}</strong>.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Search + list */}
          <div className="space-y-1.5">
            <Label>Permiso *</Label>
            <Input
              placeholder="Buscar permiso..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <div
              className="max-h-44 overflow-y-auto rounded-md border"
              style={{ borderColor: '#E0E5EC' }}
            >
              {filtered.length === 0 ? (
                <p className="text-xs text-center py-4" style={{ color: '#9CA3AF' }}>
                  Sin resultados.
                </p>
              ) : (
                filtered.map((p) => (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => setSelectedId(p.id)}
                    className="w-full text-left px-3 py-2 text-sm transition-colors"
                    style={{
                      backgroundColor: selectedId === p.id ? '#EFF6FF' : undefined,
                      color: selectedId === p.id ? '#004899' : '#374151',
                    }}
                    onMouseEnter={(e) => {
                      if (selectedId !== p.id)
                        (e.currentTarget as HTMLElement).style.backgroundColor = '#F9FAFB'
                    }}
                    onMouseLeave={(e) => {
                      if (selectedId !== p.id)
                        (e.currentTarget as HTMLElement).style.backgroundColor = ''
                    }}
                  >
                    <span className="font-mono text-xs">{p.code}</span>
                    {p.description && (
                      <span className="block text-xs truncate" style={{ color: '#6B7280' }}>
                        {p.description}
                      </span>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>

          {/* Vigencia toggle */}
          <div className="space-y-3">
            <Label>Vigencia</Label>
            <div className="flex gap-2">
              {[
                { label: 'Permanente', value: false },
                { label: 'Temporal', value: true },
              ].map(({ label, value }) => (
                <button
                  key={label}
                  type="button"
                  onClick={() => setIsTemporal(value)}
                  className="px-4 py-1.5 rounded-full text-xs font-medium transition-colors"
                  style={
                    isTemporal === value
                      ? { backgroundColor: '#004899', color: '#FFFFFF' }
                      : { backgroundColor: '#F3F4F6', color: '#374151' }
                  }
                >
                  {label}
                </button>
              ))}
            </div>

            {isTemporal && (
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <Label htmlFor="perm-modal-from" className="text-xs">Desde (opcional)</Label>
                  <Input
                    id="perm-modal-from"
                    type="datetime-local"
                    value={validFrom}
                    onChange={(e) => setValidFrom(e.target.value)}
                    className="text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="perm-modal-until" className="text-xs">Hasta *</Label>
                  <Input
                    id="perm-modal-until"
                    type="datetime-local"
                    value={validUntil}
                    onChange={(e) => setValidUntil(e.target.value)}
                    className="text-xs"
                  />
                </div>
              </div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            Cancelar
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!selectedId || (isTemporal && !validUntil) || isSubmitting}
          >
            {isSubmitting ? 'Asignando...' : 'Asignar permiso'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

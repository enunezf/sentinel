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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { rolesApi } from '@/api/roles'
import { usersApi } from '@/api/users'
import type { Role } from '@/types'
import { toast } from '@/hooks/useToast'

interface AsignarRolModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  username: string
  onSuccess: () => void
}

export function AsignarRolModal({
  open,
  onOpenChange,
  userId,
  username,
  onSuccess,
}: AsignarRolModalProps) {
  const [roles, setRoles] = useState<Role[]>([])
  const [selectedRoleId, setSelectedRoleId] = useState('')
  const [isTemporal, setIsTemporal] = useState(false)
  const [validFrom, setValidFrom] = useState('')
  const [validUntil, setValidUntil] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (open) {
      setSelectedRoleId('')
      setIsTemporal(false)
      setValidFrom('')
      setValidUntil('')
      void rolesApi.list({ page_size: 100 }).then((r) => setRoles(r.data.filter((r) => r.is_active)))
    }
  }, [open])

  const handleSubmit = async () => {
    if (!selectedRoleId) return
    setIsSubmitting(true)
    try {
      await usersApi.assignRole(userId, {
        role_id: selectedRoleId,
        valid_from: isTemporal && validFrom ? validFrom : undefined,
        valid_until: isTemporal && validUntil ? validUntil : null,
      })
      const roleName = roles.find((r) => r.id === selectedRoleId)?.name ?? ''
      toast({ title: 'Rol asignado', description: `"${roleName}" asignado a @${username}.` })
      onOpenChange(false)
      onSuccess()
    } catch {
      toast({ title: 'Error al asignar rol', variant: 'destructive' })
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Asignar rol</DialogTitle>
          <DialogDescription>
            Asignar un rol de acceso a <strong>@{username}</strong>.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Role selector */}
          <div className="space-y-1.5">
            <Label htmlFor="modal-role">Rol *</Label>
            <Select value={selectedRoleId} onValueChange={setSelectedRoleId}>
              <SelectTrigger id="modal-role">
                <SelectValue placeholder="Seleccionar rol..." />
              </SelectTrigger>
              <SelectContent>
                {roles.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
                  <Label htmlFor="modal-role-from" className="text-xs">Desde (opcional)</Label>
                  <Input
                    id="modal-role-from"
                    type="datetime-local"
                    value={validFrom}
                    onChange={(e) => setValidFrom(e.target.value)}
                    className="text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="modal-role-until" className="text-xs">Hasta *</Label>
                  <Input
                    id="modal-role-until"
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
            disabled={!selectedRoleId || (isTemporal && !validUntil) || isSubmitting}
          >
            {isSubmitting ? 'Asignando...' : 'Asignar rol'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

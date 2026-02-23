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
import { costCentersApi } from '@/api/costCenters'
import { usersApi } from '@/api/users'
import type { CostCenter } from '@/types'
import { toast } from '@/hooks/useToast'

interface AsignarCeCosModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  username: string
  onSuccess: () => void
}

export function AsignarCeCosModal({
  open,
  onOpenChange,
  userId,
  username,
  onSuccess,
}: AsignarCeCosModalProps) {
  const [costCenters, setCostCenters] = useState<CostCenter[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [isTemporal, setIsTemporal] = useState(false)
  const [validFrom, setValidFrom] = useState('')
  const [validUntil, setValidUntil] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (open) {
      setSelected([])
      setIsTemporal(false)
      setValidFrom('')
      setValidUntil('')
      void costCentersApi
        .list({ page_size: 100 })
        .then((r) => setCostCenters(r.data.filter((cc) => cc.is_active)))
    }
  }, [open])

  const toggleCc = (id: string) => {
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    )
  }

  const handleSubmit = async () => {
    if (selected.length === 0) return
    setIsSubmitting(true)
    try {
      await usersApi.assignCostCenters(userId, {
        cost_center_ids: selected,
        valid_from: isTemporal && validFrom ? validFrom : undefined,
        valid_until: isTemporal && validUntil ? validUntil : null,
      })
      toast({
        title: 'Centros de costo asignados',
        description: `${selected.length} centro(s) asignado(s) a @${username}.`,
      })
      onOpenChange(false)
      onSuccess()
    } catch {
      toast({ title: 'Error al asignar centros de costo', variant: 'destructive' })
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Asignar centros de costo</DialogTitle>
          <DialogDescription>
            Seleccione los centros de costo para <strong>@{username}</strong>.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Cost center list */}
          <div className="space-y-1.5">
            <Label>Centros de costo *</Label>
            <div
              className="max-h-52 overflow-y-auto rounded-md border divide-y"
              style={{ borderColor: '#E0E5EC' }}
            >
              {costCenters.length === 0 ? (
                <p className="text-xs text-center py-4" style={{ color: '#9CA3AF' }}>
                  No hay centros de costo disponibles.
                </p>
              ) : (
                costCenters.map((cc) => (
                  <label
                    key={cc.id}
                    className="flex items-center gap-3 px-3 py-2.5 cursor-pointer transition-colors"
                    style={{
                      backgroundColor: selected.includes(cc.id) ? '#EFF6FF' : undefined,
                    }}
                    onMouseEnter={(e) => {
                      if (!selected.includes(cc.id))
                        (e.currentTarget as HTMLElement).style.backgroundColor = '#F9FAFB'
                    }}
                    onMouseLeave={(e) => {
                      if (!selected.includes(cc.id))
                        (e.currentTarget as HTMLElement).style.backgroundColor = ''
                    }}
                  >
                    <input
                      type="checkbox"
                      checked={selected.includes(cc.id)}
                      onChange={() => toggleCc(cc.id)}
                      className="h-4 w-4 rounded border-gray-300"
                      style={{ accentColor: '#004899' }}
                    />
                    <span className="font-mono text-xs font-medium" style={{ color: '#374151' }}>
                      {cc.code}
                    </span>
                    <span className="text-sm truncate" style={{ color: '#6B7280' }}>
                      {cc.name}
                    </span>
                  </label>
                ))
              )}
            </div>
            {selected.length > 0 && (
              <p className="text-xs" style={{ color: '#004899' }}>
                {selected.length} seleccionado(s)
              </p>
            )}
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
                  <Label htmlFor="cc-modal-from" className="text-xs">Desde (opcional)</Label>
                  <Input
                    id="cc-modal-from"
                    type="datetime-local"
                    value={validFrom}
                    onChange={(e) => setValidFrom(e.target.value)}
                    className="text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="cc-modal-until" className="text-xs">Hasta *</Label>
                  <Input
                    id="cc-modal-until"
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
            disabled={selected.length === 0 || (isTemporal && !validUntil) || isSubmitting}
          >
            {isSubmitting ? 'Asignando...' : `Asignar ${selected.length > 0 ? selected.length : ''} centro(s)`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

import { useEffect, useState } from 'react'
import axios from 'axios'
import {
  Server,
  Database,
  MemoryStick,
  ExternalLink,
  RefreshCw,
  CheckCircle2,
  XCircle,
  Clock,
  ShieldCheck,
  Lock,
  KeyRound,
} from 'lucide-react'
import { PageHeader } from '@/components/shared/PageHeader'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/utils'

// ─── Types ───────────────────────────────────────────────────────────────────

interface HealthResponse {
  status: 'healthy' | 'unhealthy'
  version: string
  checks: Record<string, string>
}

// Derive backend origin from the configured API URL
const API_URL = import.meta.env.VITE_API_URL as string | undefined
const BACKEND_ORIGIN = API_URL ? API_URL.replace(/\/api\/v1\/?$/, '') : 'http://localhost:8080'

// ─── Components ──────────────────────────────────────────────────────────────

function ServiceRow({
  label,
  status,
  icon: Icon,
}: {
  label: string
  status: string | undefined
  icon: React.ElementType
}) {
  const ok = status === 'ok'
  return (
    <div
      className="flex items-center gap-3 p-3 rounded-lg"
      style={{ backgroundColor: ok ? '#F0FDF4' : '#FEF2F2', border: `1px solid ${ok ? '#BBF7D0' : '#FECACA'}` }}
    >
      <Icon className="h-4 w-4 shrink-0" style={{ color: ok ? '#16A34A' : '#D0021B' }} />
      <div className="flex-1">
        <p className="text-sm font-medium" style={{ color: '#1A1A2E' }}>{label}</p>
        {!ok && status && (
          <p className="text-xs mt-0.5" style={{ color: '#D0021B' }}>{status}</p>
        )}
      </div>
      {ok ? (
        <CheckCircle2 className="h-4 w-4 shrink-0" style={{ color: '#16A34A' }} />
      ) : (
        <XCircle className="h-4 w-4 shrink-0" style={{ color: '#D0021B' }} />
      )}
    </div>
  )
}

function PolicyItem({ icon: Icon, label, value }: { icon: React.ElementType; label: string; value: string }) {
  return (
    <div className="flex items-start gap-3 py-3" style={{ borderBottom: '1px solid #F3F4F6' }}>
      <div
        className="h-7 w-7 rounded-lg flex items-center justify-center shrink-0 mt-0.5"
        style={{ backgroundColor: '#EFF6FF' }}
      >
        <Icon className="h-3.5 w-3.5" style={{ color: '#004899' }} />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium" style={{ color: '#6B7280' }}>{label}</p>
        <p className="text-sm mt-0.5" style={{ color: '#1A1A2E' }}>{value}</p>
      </div>
    </div>
  )
}

// ─── Page ────────────────────────────────────────────────────────────────────

export function SystemConfigPage() {
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [lastChecked, setLastChecked] = useState<Date | null>(null)
  const [error, setError] = useState<string | null>(null)

  const fetchHealth = async () => {
    setIsLoading(true)
    setError(null)
    try {
      const res = await axios.get<HealthResponse>(`${BACKEND_ORIGIN}/health`, { timeout: 5000 })
      setHealth(res.data)
      setLastChecked(new Date())
    } catch {
      setError('No se pudo conectar con el servidor. Verifique que el servicio esté corriendo.')
      setHealth(null)
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => { void fetchHealth() }, [])

  const overallOk = health?.status === 'healthy'

  return (
    <div>
      <PageHeader
        title="Configuración del Sistema"
        description="Estado del servicio y políticas de seguridad"
        actions={
          <Button variant="outline" onClick={() => { void fetchHealth() }} disabled={isLoading}>
            <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
            Actualizar
          </Button>
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* ── Service Status ── */}
        <div
          className="rounded-xl p-5 space-y-4"
          style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
        >
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div
                className="h-8 w-8 rounded-lg flex items-center justify-center"
                style={{ backgroundColor: '#EFF6FF' }}
              >
                <Server className="h-4 w-4" style={{ color: '#004899' }} />
              </div>
              <h3 className="font-semibold text-sm" style={{ color: '#1A1A2E' }}>
                Estado del Servicio
              </h3>
            </div>

            {/* Overall status pill */}
            {health && (
              <span
                className="text-xs px-2.5 py-1 rounded-full font-semibold"
                style={
                  overallOk
                    ? { backgroundColor: '#DCFCE7', color: '#15803D' }
                    : { backgroundColor: '#FEE2E2', color: '#D0021B' }
                }
              >
                {overallOk ? 'Operativo' : 'Degradado'}
              </span>
            )}
          </div>

          {isLoading && !health && (
            <div className="flex items-center justify-center py-8 gap-2" style={{ color: '#9CA3AF' }}>
              <RefreshCw className="h-4 w-4 animate-spin" />
              <span className="text-sm">Verificando servicios...</span>
            </div>
          )}

          {error && (
            <div
              className="flex items-center gap-2 text-sm rounded-lg p-3"
              style={{ backgroundColor: '#FEF2F2', border: '1px solid #FECACA', color: '#D0021B' }}
            >
              <XCircle className="h-4 w-4 shrink-0" />
              {error}
            </div>
          )}

          {health && (
            <div className="space-y-2">
              <ServiceRow
                label="PostgreSQL"
                status={health.checks.postgresql}
                icon={Database}
              />
              <ServiceRow
                label="Redis"
                status={health.checks.redis}
                icon={MemoryStick}
              />
            </div>
          )}

          {lastChecked && (
            <p className="text-xs flex items-center gap-1" style={{ color: '#9CA3AF' }}>
              <Clock className="h-3 w-3" />
              Última verificación: {formatDate(lastChecked.toISOString())}
            </p>
          )}
        </div>

        {/* ── Version & Endpoints ── */}
        <div
          className="rounded-xl p-5 space-y-4"
          style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
        >
          <div className="flex items-center gap-2">
            <div
              className="h-8 w-8 rounded-lg flex items-center justify-center"
              style={{ backgroundColor: '#EFF6FF' }}
            >
              <KeyRound className="h-4 w-4" style={{ color: '#004899' }} />
            </div>
            <h3 className="font-semibold text-sm" style={{ color: '#1A1A2E' }}>
              API y Claves
            </h3>
          </div>

          <div className="space-y-3">
            <div
              className="flex items-center justify-between p-3 rounded-lg"
              style={{ backgroundColor: '#F4F6F9' }}
            >
              <div>
                <p className="text-xs font-medium" style={{ color: '#6B7280' }}>Versión del servicio</p>
                <p className="text-sm font-semibold font-mono" style={{ color: '#1A1A2E' }}>
                  v{health?.version ?? '—'}
                </p>
              </div>
            </div>

            <div
              className="flex items-center justify-between p-3 rounded-lg"
              style={{ backgroundColor: '#F4F6F9' }}
            >
              <div className="min-w-0 flex-1">
                <p className="text-xs font-medium" style={{ color: '#6B7280' }}>JWKS Endpoint</p>
                <p
                  className="text-xs font-mono truncate"
                  style={{ color: '#374151' }}
                >
                  {BACKEND_ORIGIN}/.well-known/jwks.json
                </p>
              </div>
              <a
                href={`${BACKEND_ORIGIN}/.well-known/jwks.json`}
                target="_blank"
                rel="noopener noreferrer"
                className="ml-2 shrink-0 p-1.5 rounded-lg transition-colors"
                style={{ color: '#004899' }}
                onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = '#EFF6FF' }}
                onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent' }}
                title="Abrir JWKS"
              >
                <ExternalLink className="h-4 w-4" />
              </a>
            </div>

            <div
              className="flex items-center justify-between p-3 rounded-lg"
              style={{ backgroundColor: '#F4F6F9' }}
            >
              <div className="min-w-0 flex-1">
                <p className="text-xs font-medium" style={{ color: '#6B7280' }}>Health Endpoint</p>
                <p className="text-xs font-mono truncate" style={{ color: '#374151' }}>
                  {BACKEND_ORIGIN}/health
                </p>
              </div>
              <a
                href={`${BACKEND_ORIGIN}/health`}
                target="_blank"
                rel="noopener noreferrer"
                className="ml-2 shrink-0 p-1.5 rounded-lg transition-colors"
                style={{ color: '#004899' }}
                onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = '#EFF6FF' }}
                onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent' }}
                title="Abrir health"
              >
                <ExternalLink className="h-4 w-4" />
              </a>
            </div>
          </div>
        </div>

        {/* ── Security Policies ── */}
        <div
          className="lg:col-span-2 rounded-xl p-5"
          style={{ backgroundColor: '#FFFFFF', border: '1px solid #E0E5EC' }}
        >
          <div className="flex items-center gap-2 mb-4">
            <div
              className="h-8 w-8 rounded-lg flex items-center justify-center"
              style={{ backgroundColor: '#EFF6FF' }}
            >
              <ShieldCheck className="h-4 w-4" style={{ color: '#004899' }} />
            </div>
            <div>
              <h3 className="font-semibold text-sm" style={{ color: '#1A1A2E' }}>
                Políticas de Seguridad
              </h3>
              <p className="text-xs" style={{ color: '#6B7280' }}>
                Configuración activa del servicio (solo lectura)
              </p>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-8">
            <div>
              <PolicyItem icon={Lock} label="Algoritmo JWT" value="RS256 (RSA + SHA-256)" />
              <PolicyItem icon={Clock} label="TTL Access Token (web)" value="15 minutos" />
              <PolicyItem icon={Clock} label="TTL Refresh Token (web)" value="7 días" />
              <PolicyItem icon={Clock} label="TTL Refresh Token (mobile/desktop)" value="30 días" />
            </div>
            <div>
              <PolicyItem icon={ShieldCheck} label="Intentos máximos de login" value="5 por período" />
              <PolicyItem icon={Lock} label="Bloqueo temporal (1–2 fallas)" value="15 minutos" />
              <PolicyItem icon={Lock} label="Bloqueo temporal (3+ fallas/día)" value="Permanente (manual unlock)" />
              <PolicyItem icon={ShieldCheck} label="Historial de contraseñas" value="Últimas 5 no reutilizables" />
            </div>
            <div>
              <PolicyItem icon={Lock} label="Hash de contraseñas" value="bcrypt (costo ≥ 12)" />
              <PolicyItem icon={ShieldCheck} label="Normalización de contraseñas" value="Unicode NFC" />
              <PolicyItem icon={KeyRound} label="Longitud mínima de contraseña" value="10 caracteres" />
              <PolicyItem icon={ShieldCheck} label="Política de complejidad" value="Mayúscula + número + símbolo" />
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

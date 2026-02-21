/**
 * Sentinel k6 Global Configuration
 *
 * Centraliza opciones comunes para todos los escenarios de carga.
 * Uso: importar en cada scenario file o ejecutar con:
 *
 *   k6 run --config tests/load/k6.config.js tests/load/scenarios/<scenario>.js
 *
 * Variables de entorno requeridas:
 *   BASE_URL  - URL base del servicio (default: http://localhost:8080)
 *   APP_KEY   - Secret key de la aplicación de prueba
 *
 * Variables opcionales:
 *   TEST_USERNAME       - Usuario para pruebas de login
 *   TEST_PASSWORD       - Contraseña para pruebas de login
 *   TEST_ADMIN_USERNAME - Usuario admin (para mixed_load)
 *   TEST_ADMIN_PASSWORD - Contraseña admin (para mixed_load)
 *   TEST_PERMISSION     - Código de permiso a verificar (authz_verify_load)
 *   ACCESS_TOKEN        - Token pre-obtenido (opcional, evita el setup())
 */

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const APP_KEY  = __ENV.APP_KEY  || '';

// Global thresholds applied to ALL scenarios unless overridden per-scenario.
export const options = {
  // ---- Global thresholds ----
  thresholds: {
    http_req_duration:             ['p(95)<200'],  // Global p95 SLA
    http_req_failed:               ['rate<0.01'],  // Max 1% errors
    'http_req_duration{name:login}':  ['p(95)<200'],
    'http_req_duration{name:authz}':  ['p(95)<50'],
    'http_req_duration{name:admin}':  ['p(95)<500'],
  },

  // ---- All scenarios ----
  scenarios: {
    // Login ramp: 0 -> 50 VUs in 30s, sustain 2m, down 30s.
    login_ramp: {
      executor: 'ramping-vus',
      exec:     'loginScenario',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 50 },
        { duration: '2m',  target: 50 },
        { duration: '30s', target: 0  },
      ],
      gracefulRampDown: '10s',
      tags: { scenario: 'login' },
    },

    // Authz sustained: 100 VUs for 3 minutes.
    authz_sustained: {
      executor: 'constant-vus',
      exec:     'authzScenario',
      vus:      100,
      duration: '3m',
      tags:     { scenario: 'authz' },
    },

    // Mixed realistic: 500 VUs for 5 minutes.
    mixed_realistic: {
      executor: 'constant-vus',
      exec:     'mixedScenario',
      vus:      500,
      duration: '5m',
      tags:     { scenario: 'mixed' },
    },
  },

  // ---- Common HTTP settings ----
  httpDebug: 'false',
  noConnectionReuse: false,
  userAgent: 'Sentinel-k6-LoadTest/1.0',
};

// ---------- Scenario function stubs ----------
// These are meant to be exported from individual scenario files.
// When using k6.config.js as the entry point, import and re-export them.

export function loginScenario() {
  const loginModule = require('./scenarios/login_load.js');
  loginModule.default();
}

export function authzScenario() {
  const authzModule = require('./scenarios/authz_verify_load.js');
  authzModule.default();
}

export function mixedScenario() {
  const mixedModule = require('./scenarios/mixed_load.js');
  mixedModule.default();
}

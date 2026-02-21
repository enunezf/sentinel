/**
 * Sentinel Load Test - Mixed Realistic Scenario
 *
 * Mezcla realista:
 *   - 70% authz/verify  (backends verificando permisos por request)
 *   - 20% login         (usuarios autenticándose)
 *   - 10% admin/users   (operadores administrando usuarios)
 *
 * 500 VUs concurrentes (SLA: 500+ usuarios concurrentes del spec)
 * Duración: 5 minutos
 *
 * Thresholds globales:
 *   - http_req_duration: p95 < 200ms
 *   - http_req_failed: rate < 1%
 */

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const APP_KEY  = __ENV.APP_KEY  || 'test-app-key';

// Custom metrics per operation type.
const authzRequests  = new Counter('authz_requests');
const loginRequests  = new Counter('login_requests');
const adminRequests  = new Counter('admin_requests');
const authzSuccess   = new Rate('authz_success_rate');
const loginSuccess   = new Rate('login_success_rate');

export const options = {
  scenarios: {
    mixed_500vus: {
      executor:  'constant-vus',
      vus:       500,
      duration:  '5m',
    },
  },
  thresholds: {
    // Global p95 < 200ms across all request types.
    'http_req_duration':             ['p(95)<200'],
    'http_req_failed':               ['rate<0.01'],
    // Per-operation thresholds.
    'http_req_duration{name:authz}': ['p(95)<50'],
    'http_req_duration{name:login}': ['p(95)<200'],
    'http_req_duration{name:admin}': ['p(95)<500'],
    // Success rates.
    'authz_success_rate':            ['rate>0.99'],
    'login_success_rate':            ['rate>0.99'],
  },
};

let sharedAccessToken = null;
let sharedAdminToken  = null;

export function setup() {
  const loginPayload = JSON.stringify({
    username:    __ENV.TEST_USERNAME || 'testuser',
    password:    __ENV.TEST_PASSWORD || 'S3cur3P@ss!',
    client_type: 'web',
  });

  const headers = {
    'Content-Type': 'application/json',
    'X-App-Key':    APP_KEY,
  };

  // Regular user token.
  const userRes = http.post(`${BASE_URL}/api/v1/auth/login`, loginPayload, { headers });
  const accessToken = userRes.status === 200 ? userRes.json('access_token') : null;

  // Admin token (same credentials for simplicity; use TEST_ADMIN_* env vars in real run).
  const adminPayload = JSON.stringify({
    username:    __ENV.TEST_ADMIN_USERNAME || __ENV.TEST_USERNAME || 'admin',
    password:    __ENV.TEST_ADMIN_PASSWORD || __ENV.TEST_PASSWORD || 'S3cur3P@ss!',
    client_type: 'web',
  });
  const adminRes    = http.post(`${BASE_URL}/api/v1/auth/login`, adminPayload, { headers });
  const adminToken  = adminRes.status === 200 ? adminRes.json('access_token') : null;

  return { accessToken, adminToken };
}

export default function (data) {
  const accessToken = data.accessToken || '';
  const adminToken  = data.adminToken  || '';

  // Weighted random: 70% authz, 20% login, 10% admin.
  const roll = Math.random();

  if (roll < 0.70) {
    // --- authz/verify (70%) ---
    group('authz_verify', () => {
      authzRequests.add(1);

      const res = http.post(
        `${BASE_URL}/api/v1/authz/verify`,
        JSON.stringify({ permission: 'inventory.stock.read' }),
        {
          headers: {
            'Content-Type':  'application/json',
            'Authorization': `Bearer ${accessToken}`,
            'X-App-Key':     APP_KEY,
          },
          tags: { name: 'authz' },
        }
      );

      const ok = check(res, {
        'authz 200':            (r) => r.status === 200,
        'authz has allowed':    (r) => typeof r.json('allowed') === 'boolean',
      });
      authzSuccess.add(ok ? 1 : 0);
    });

    sleep(0.05);

  } else if (roll < 0.90) {
    // --- login (20%) ---
    group('login', () => {
      loginRequests.add(1);

      const res = http.post(
        `${BASE_URL}/api/v1/auth/login`,
        JSON.stringify({
          username:    __ENV.TEST_USERNAME || 'testuser',
          password:    __ENV.TEST_PASSWORD || 'S3cur3P@ss!',
          client_type: 'web',
        }),
        {
          headers: {
            'Content-Type': 'application/json',
            'X-App-Key':    APP_KEY,
          },
          tags: { name: 'login' },
        }
      );

      const ok = check(res, {
        'login 200':              (r) => r.status === 200,
        'login has access_token': (r) => (r.json('access_token') || '') !== '',
      });
      loginSuccess.add(ok ? 1 : 0);
    });

    sleep(Math.random() * 0.3 + 0.1);

  } else {
    // --- admin/users (10%) ---
    group('admin_users', () => {
      adminRequests.add(1);

      const res = http.get(
        `${BASE_URL}/api/v1/admin/users?page=1&page_size=20`,
        {
          headers: {
            'Authorization': `Bearer ${adminToken}`,
            'X-App-Key':     APP_KEY,
          },
          tags: { name: 'admin' },
        }
      );

      check(res, {
        'admin 200':           (r) => r.status === 200,
        'admin has data':      (r) => Array.isArray(r.json('data')),
        'admin has total':     (r) => typeof r.json('total') === 'number',
      });
    });

    sleep(Math.random() * 0.5 + 0.2);
  }
}

export function teardown(data) {
  // No cleanup required.
  console.log(`Mixed load test complete. Tokens acquired: ${data.accessToken ? 'yes' : 'no'}`);
}

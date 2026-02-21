/**
 * Sentinel Load Test - Authorization Verify Scenario
 *
 * SLA objetivo: POST /authz/verify < 50ms p95
 *
 * Escenario:
 *   - 100 VUs sostenidos por 3 minutos (simula backends consumidores)
 *
 * Thresholds:
 *   - http_req_duration{name:'authz'}: p95 < 50ms
 *   - http_req_failed: rate < 1%
 *
 * Prerequisito: ACCESS_TOKEN debe estar definido como env var, o el setup
 * realiza un login para obtenerlo.
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { SharedArray } from 'k6/data';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const APP_KEY  = __ENV.APP_KEY  || 'test-app-key';

export const options = {
  scenarios: {
    authz_sustained: {
      executor:  'constant-vus',
      vus:       100,
      duration:  '3m',
    },
  },
  thresholds: {
    'http_req_duration{name:authz}': ['p(95)<50'],
    'http_req_failed':               ['rate<0.01'],
  },
};

// Shared access token obtained via setup() and shared across VUs.
let sharedAccessToken = null;

export function setup() {
  // Obtain a valid access token for verify calls.
  const loginPayload = JSON.stringify({
    username:    __ENV.TEST_USERNAME || 'testuser',
    password:    __ENV.TEST_PASSWORD || 'S3cur3P@ss!',
    client_type: 'web',
  });

  const res = http.post(`${BASE_URL}/api/v1/auth/login`, loginPayload, {
    headers: {
      'Content-Type': 'application/json',
      'X-App-Key':    APP_KEY,
    },
  });

  if (res.status !== 200) {
    console.error(`setup: login failed status=${res.status} body=${res.body}`);
    return { accessToken: null };
  }

  return { accessToken: res.json('access_token') };
}

export default function (data) {
  const accessToken = data.accessToken || __ENV.ACCESS_TOKEN || '';

  if (!accessToken) {
    console.warn('No access token available for authz/verify test');
    return;
  }

  const payload = JSON.stringify({
    permission: __ENV.TEST_PERMISSION || 'inventory.stock.read',
  });

  const params = {
    headers: {
      'Content-Type':  'application/json',
      'Authorization': `Bearer ${accessToken}`,
      'X-App-Key':     APP_KEY,
    },
    tags: { name: 'authz' },
  };

  const res = http.post(`${BASE_URL}/api/v1/authz/verify`, payload, params);

  check(res, {
    'authz verify status 200':    (r) => r.status === 200,
    'authz response has allowed': (r) => typeof r.json('allowed') === 'boolean',
    'authz has user_id':          (r) => r.json('user_id') !== '',
    'authz has evaluated_at':     (r) => r.json('evaluated_at') !== '',
  });

  // Very short pause — backends call /authz/verify per request.
  sleep(0.05);
}

/**
 * Sentinel Load Test - Login Scenario
 *
 * SLA objetivo: POST /auth/login < 200ms p95
 *
 * Escenario:
 *   - Rampa de 0 -> 50 VUs en 30s
 *   - Mantener 50 VUs por 2 min
 *   - Bajar a 0 en 30s
 *
 * Thresholds:
 *   - http_req_duration{name:'login'}: p95 < 200ms
 *   - http_req_failed: rate < 1%
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const APP_KEY  = __ENV.APP_KEY  || 'test-app-key';

// Custom metrics.
const loginErrors   = new Counter('login_errors');
const loginDuration = new Trend('login_duration', true);

export const options = {
  scenarios: {
    login_ramp: {
      executor:          'ramping-vus',
      startVUs:          0,
      stages: [
        { duration: '30s', target: 50 },   // ramp up
        { duration: '2m',  target: 50 },   // sustain
        { duration: '30s', target: 0  },   // ramp down
      ],
      gracefulRampDown: '10s',
    },
  },
  thresholds: {
    'http_req_duration{name:login}': ['p(95)<200'],
    'http_req_failed':               ['rate<0.01'],
  },
};

const CREDENTIALS = {
  username:    __ENV.TEST_USERNAME    || 'testuser',
  password:    __ENV.TEST_PASSWORD    || 'S3cur3P@ss!',
  client_type: 'web',
};

export default function () {
  const payload = JSON.stringify(CREDENTIALS);

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-App-Key':    APP_KEY,
    },
    tags: { name: 'login' },
  };

  const start = Date.now();
  const res   = http.post(`${BASE_URL}/api/v1/auth/login`, payload, params);
  loginDuration.add(Date.now() - start);

  const ok = check(res, {
    'login status 200':         (r) => r.status === 200,
    'login has access_token':   (r) => r.json('access_token') !== '',
    'login has refresh_token':  (r) => r.json('refresh_token') !== '',
    'login token_type=Bearer':  (r) => r.json('token_type') === 'Bearer',
    'login expires_in=3600':    (r) => r.json('expires_in') === 3600,
  });

  if (!ok) {
    loginErrors.add(1);
  }

  // Brief pause to avoid thundering-herd.
  sleep(Math.random() * 0.5 + 0.1);
}

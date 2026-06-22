import exec from 'k6/execution';
import http from 'k6/http';
import { fail, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const baseURL = trimTrailingSlashes(__ENV.NEXUSPAAS_PERF_BASE_URL || 'http://127.0.0.1:18080');
const apiKeys = parseAPIKeys();
const vus = Number.parseInt(__ENV.NEXUSPAAS_PERF_VUS || '100', 10);
const duration = __ENV.NEXUSPAAS_PERF_DURATION || '30s';
const timeout = __ENV.NEXUSPAAS_PERF_TIMEOUT || '5s';
const thinkTime = Number.parseFloat(__ENV.NEXUSPAAS_PERF_THINK_TIME || '1');
const summaryPath = __ENV.NEXUSPAAS_PERF_SUMMARY_PATH || '.tmp/nexuspaas-perf-core-read-100vu-endpoints.json';

const endpoints = [
  { path: '/healthz', expectedStatus: 200, protected: false },
  { path: '/readyz', expectedStatus: 200, protected: false },
  { path: '/api/v1/projects', expectedStatus: 200, protected: true },
];

const status2xx = new Counter('nexuspaas_perf_status_2xx');
const status4xx = new Counter('nexuspaas_perf_status_4xx');
const status5xx = new Counter('nexuspaas_perf_status_5xx');
const status429 = new Counter('nexuspaas_perf_status_429');
const statusOther = new Counter('nexuspaas_perf_status_other');

export const options = {
  scenarios: {
    core_read: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '5s',
    },
  },
  thresholds: {
    http_req_failed: [
      'rate<0.01',
      { threshold: 'rate<0.05', abortOnFail: true, delayAbortEval: '5s' },
    ],
    'http_req_failed{endpoint:/healthz}': ['rate<0.01'],
    'http_req_failed{endpoint:/readyz}': ['rate<0.01'],
    'http_req_failed{endpoint:/api/v1/projects}': ['rate<0.01'],
    'http_reqs{endpoint:/healthz}': ['count>0'],
    'http_reqs{endpoint:/readyz}': ['count>0'],
    'http_reqs{endpoint:/api/v1/projects}': ['count>0'],
    'nexuspaas_perf_status_2xx{endpoint:/healthz}': ['count>=0'],
    'nexuspaas_perf_status_2xx{endpoint:/readyz}': ['count>=0'],
    'nexuspaas_perf_status_2xx{endpoint:/api/v1/projects}': ['count>=0'],
    'nexuspaas_perf_status_4xx{endpoint:/healthz}': ['count>=0'],
    'nexuspaas_perf_status_4xx{endpoint:/readyz}': ['count>=0'],
    'nexuspaas_perf_status_4xx{endpoint:/api/v1/projects}': ['count>=0'],
    'nexuspaas_perf_status_5xx{endpoint:/healthz}': ['count>=0'],
    'nexuspaas_perf_status_5xx{endpoint:/readyz}': ['count>=0'],
    'nexuspaas_perf_status_5xx{endpoint:/api/v1/projects}': ['count>=0'],
    'nexuspaas_perf_status_429{endpoint:/healthz}': ['count>=0'],
    'nexuspaas_perf_status_429{endpoint:/readyz}': ['count>=0'],
    'nexuspaas_perf_status_429{endpoint:/api/v1/projects}': ['count>=0'],
    'nexuspaas_perf_status_other{endpoint:/healthz}': ['count>=0'],
    'nexuspaas_perf_status_other{endpoint:/readyz}': ['count>=0'],
    'nexuspaas_perf_status_other{endpoint:/api/v1/projects}': ['count>=0'],
    'http_req_duration{endpoint:/healthz}': ['p(95)<1000'],
    'http_req_duration{endpoint:/readyz}': ['p(95)<1000'],
    'http_req_duration{endpoint:/api/v1/projects}': ['p(95)<300'],
  },
};

function requestParams(endpoint, phase = 'load') {
  const headers = { Accept: 'application/json' };
  if (endpoint.protected) {
    headers['X-API-Key'] = apiKeyForRequest();
  }
  const endpointTag = phase === 'preflight' ? `preflight:${endpoint.path}` : endpoint.path;
  return {
    headers,
    timeout,
    tags: { endpoint: endpointTag, phase },
  };
}

function requireSetup() {
  if (apiKeys.length === 0) {
    fail('NEXUSPAAS_PERF_API_KEYS or NEXUSPAAS_PERF_API_KEY is required for /api/v1/projects');
  }
}

export function setup() {
  requireSetup();

  for (const endpoint of endpoints) {
    if (endpoint.protected) {
      for (const key of apiKeys) {
        const res = http.get(`${baseURL}${endpoint.path}`, requestParamsWithKey(endpoint, key, 'preflight'));
        if (res.status !== endpoint.expectedStatus) {
          fail(`preflight ${endpoint.path} returned ${res.status}, want ${endpoint.expectedStatus}`);
        }
      }
      continue;
    }
    const res = http.get(`${baseURL}${endpoint.path}`, requestParams(endpoint, 'preflight'));
    if (res.status !== endpoint.expectedStatus) {
      fail(`preflight ${endpoint.path} returned ${res.status}, want ${endpoint.expectedStatus}`);
    }
  }
}

export default function coreReadScenario() {
  const endpoint = endpoints[exec.scenario.iterationInTest % endpoints.length];
  const res = http.get(`${baseURL}${endpoint.path}`, requestParams(endpoint));
  recordStatus(endpoint, res.status);
  if (thinkTime > 0) {
    sleep(thinkTime);
  }
}

export function handleSummary(data) {
  const result = {
    scenario: 'core-read-100vu',
    config: {
      base_url: baseURL,
      vus,
      duration,
      timeout,
      think_time: thinkTime,
      auth_key_count: apiKeys.length,
    },
    totals: {
      requests: metricValue(data, 'http_reqs', 'count'),
      failure_rate: metricValue(data, 'http_req_failed', 'rate'),
      p95_ms: metricValue(data, 'http_req_duration', 'p(95)'),
    },
    endpoints: {},
  };

  for (const endpoint of endpoints) {
    const requestCount = metricValue(data, `http_reqs{endpoint:${endpoint.path}}`, 'count');
    const failureRate = metricValue(data, `http_req_failed{endpoint:${endpoint.path}}`, 'rate');
    result.endpoints[endpoint.path] = {
      requests: requestCount,
      failure_rate: failureRate,
      failures: requestCount !== null && failureRate !== null ? Math.round(requestCount * failureRate) : null,
      p95_ms: metricValue(data, `http_req_duration{endpoint:${endpoint.path}}`, 'p(95)'),
      status_counts: {
        '2xx': counterValue(data, `nexuspaas_perf_status_2xx{endpoint:${endpoint.path}}`),
        '4xx': counterValue(data, `nexuspaas_perf_status_4xx{endpoint:${endpoint.path}}`),
        '5xx': counterValue(data, `nexuspaas_perf_status_5xx{endpoint:${endpoint.path}}`),
        '429': counterValue(data, `nexuspaas_perf_status_429{endpoint:${endpoint.path}}`),
        other: counterValue(data, `nexuspaas_perf_status_other{endpoint:${endpoint.path}}`),
      },
    };
  }

  return {
    [summaryPath]: JSON.stringify(result, null, 2),
  };
}

function metricValue(data, name, value) {
	const metric = data.metrics[name];
	const metricTargetValue = metric?.values?.[value];
	if (metricTargetValue === undefined) {
		return null;
	}
	return metricTargetValue;
}

function trimTrailingSlashes(value) {
	let out = value;
	while (out.endsWith('/')) {
		out = out.slice(0, -1);
	}
	return out;
}

function counterValue(data, name) {
  const value = metricValue(data, name, 'count');
  return value === null ? 0 : value;
}

function recordStatus(endpoint, status) {
  const tags = { endpoint: endpoint.path };
  if (status >= 200 && status < 300) {
    status2xx.add(1, tags);
    return;
  }
  if (status === 429) {
    status429.add(1, tags);
  }
  if (status >= 400 && status < 500) {
    status4xx.add(1, tags);
    return;
  }
  if (status >= 500 && status < 600) {
    status5xx.add(1, tags);
    return;
  }
  statusOther.add(1, tags);
}

function parseAPIKeys() {
  const multi = (__ENV.NEXUSPAAS_PERF_API_KEYS || '')
    .split(',')
    .map((value) => value.trim())
    .filter((value) => value.length > 0);
  if (multi.length > 0) {
    return multi;
  }
  const single = (__ENV.NEXUSPAAS_PERF_API_KEY || '').trim();
  return single ? [single] : [];
}

function apiKeyForRequest() {
  if (apiKeys.length === 0) {
    return '';
  }
  const vuID = exec.vu.idInTest || 1;
  return apiKeys[(vuID - 1) % apiKeys.length];
}

function requestParamsWithKey(endpoint, key, phase) {
  const headers = { Accept: 'application/json' };
  if (endpoint.protected) {
    headers['X-API-Key'] = key;
  }
  const endpointTag = phase === 'preflight' ? `preflight:${endpoint.path}` : endpoint.path;
  return {
    headers,
    timeout,
    tags: { endpoint: endpointTag, phase },
  };
}

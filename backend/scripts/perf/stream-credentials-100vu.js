import exec from 'k6/execution';
import http from 'k6/http';
import { fail, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const endpointPath = '/api/v1/stream/credentials';
const baseURL = trimTrailingSlashes(__ENV.NEXUSPAAS_PERF_BASE_URL || 'http://127.0.0.1:18080');
const apiKeys = parseAPIKeys();
const jobID = (__ENV.NEXUSPAAS_PERF_STREAM_JOB_ID || '').trim();
const vus = Number.parseInt(__ENV.NEXUSPAAS_PERF_VUS || '100', 10);
const duration = __ENV.NEXUSPAAS_PERF_DURATION || '30s';
const timeout = __ENV.NEXUSPAAS_PERF_TIMEOUT || '5s';
const thinkTime = Number.parseFloat(__ENV.NEXUSPAAS_PERF_THINK_TIME || '1');
const ttlSeconds = Number.parseInt(__ENV.NEXUSPAAS_PERF_STREAM_TTL_SECONDS || '300', 10);
const summaryPath = __ENV.NEXUSPAAS_PERF_SUMMARY_PATH || '.tmp/nexuspaas-perf-stream-credentials-100vu.json';

const status2xx = new Counter('nexuspaas_perf_status_2xx');
const status4xx = new Counter('nexuspaas_perf_status_4xx');
const status5xx = new Counter('nexuspaas_perf_status_5xx');
const status429 = new Counter('nexuspaas_perf_status_429');
const statusOther = new Counter('nexuspaas_perf_status_other');

export const options = {
  scenarios: {
    stream_credentials: {
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
    [`http_req_failed{endpoint:${endpointPath}}`]: ['rate<0.01'],
    [`http_reqs{endpoint:${endpointPath}}`]: ['count>0'],
    [`http_req_duration{endpoint:${endpointPath}}`]: ['p(95)<300'],
    [`nexuspaas_perf_status_2xx{endpoint:${endpointPath}}`]: ['count>0'],
    [`nexuspaas_perf_status_4xx{endpoint:${endpointPath}}`]: ['count>=0'],
    [`nexuspaas_perf_status_5xx{endpoint:${endpointPath}}`]: ['count>=0'],
    [`nexuspaas_perf_status_429{endpoint:${endpointPath}}`]: ['count>=0'],
    [`nexuspaas_perf_status_other{endpoint:${endpointPath}}`]: ['count>=0'],
  },
};

export function setup() {
  requireSetup();
  for (const key of apiKeys) {
    const res = http.post(`${baseURL}${endpointPath}`, requestBody('preflight'), requestParamsWithKey(key, 'preflight'));
    if (res.status !== 200) {
      fail(`preflight ${endpointPath} returned ${res.status}, want 200`);
    }
  }
}

export default function streamCredentialsScenario() {
  const res = http.post(`${baseURL}${endpointPath}`, requestBody(sessionID()), requestParams(apiKeyForRequest()));
  recordStatus(res.status);
  if (thinkTime > 0) {
    sleep(thinkTime);
  }
}

export function handleSummary(data) {
  const requestCount = metricValue(data, `http_reqs{endpoint:${endpointPath}}`, 'count');
  const failureRate = metricValue(data, `http_req_failed{endpoint:${endpointPath}}`, 'rate');
  const result = {
    scenario: 'stream-credentials-100vu',
    config: {
      base_url: baseURL,
      endpoint: endpointPath,
      vus,
      duration,
      timeout,
      think_time: thinkTime,
      ttl_seconds: ttlSeconds,
      auth_key_count: apiKeys.length,
      stream_job_id: jobID,
    },
    totals: {
      requests: metricValue(data, 'http_reqs', 'count'),
      failure_rate: metricValue(data, 'http_req_failed', 'rate'),
      p95_ms: metricValue(data, 'http_req_duration', 'p(95)'),
    },
    endpoints: {
      [endpointPath]: {
        requests: requestCount,
        failure_rate: failureRate,
        failures: requestCount !== null && failureRate !== null ? Math.round(requestCount * failureRate) : null,
        p95_ms: metricValue(data, `http_req_duration{endpoint:${endpointPath}}`, 'p(95)'),
        status_counts: {
          '2xx': counterValue(data, `nexuspaas_perf_status_2xx{endpoint:${endpointPath}}`),
          '4xx': counterValue(data, `nexuspaas_perf_status_4xx{endpoint:${endpointPath}}`),
          '5xx': counterValue(data, `nexuspaas_perf_status_5xx{endpoint:${endpointPath}}`),
          '429': counterValue(data, `nexuspaas_perf_status_429{endpoint:${endpointPath}}`),
          other: counterValue(data, `nexuspaas_perf_status_other{endpoint:${endpointPath}}`),
        },
      },
    },
  };

  return {
    [summaryPath]: JSON.stringify(result, null, 2),
  };
}

function requireSetup() {
  if (apiKeys.length === 0) {
    fail('NEXUSPAAS_PERF_API_KEYS or NEXUSPAAS_PERF_API_KEY is required');
  }
  if (!jobID) {
    fail('NEXUSPAAS_PERF_STREAM_JOB_ID is required');
  }
}

function requestBody(session) {
  return JSON.stringify({
    job_id: jobID,
    session_id: session,
    ttl_seconds: ttlSeconds,
  });
}

function requestParams(key) {
  return requestParamsWithKey(key, 'load');
}

function requestParamsWithKey(key, phase) {
  const headers = {
    Accept: 'application/json',
    'Content-Type': 'application/json',
    'X-API-Key': key,
  };
  const endpointTag = phase === 'preflight' ? `preflight:${endpointPath}` : endpointPath;
  return {
    headers,
    timeout,
    tags: { endpoint: endpointTag, phase },
  };
}

function sessionID() {
  return `k6-vu${exec.vu.idInTest || 0}-iter${exec.scenario.iterationInTest || 0}`;
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

function metricValue(data, name, value) {
  const metric = data.metrics[name];
  const metricTargetValue = metric?.values?.[value];
  if (metricTargetValue === undefined) {
    return null;
  }
  return metricTargetValue;
}

function counterValue(data, name) {
  const value = metricValue(data, name, 'count');
  return value === null ? 0 : value;
}

function recordStatus(status) {
  const tags = { endpoint: endpointPath };
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

function trimTrailingSlashes(value) {
  let out = value;
  while (out.endsWith('/')) {
    out = out.slice(0, -1);
  }
  return out;
}

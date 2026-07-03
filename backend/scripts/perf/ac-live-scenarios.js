// PERF-003/004/006/008 live scenarios against the kind-deployed 8-unit stack.
// Each scenario targets its owning unit directly through a port-forward whose
// base URL arrives via env; a 4xx (quota/authz rejection) is a valid,
// correctly-degraded answer under load — only 5xx/network failures and the
// latency budget fail the run.
//
//   k6 run backend/scripts/perf/ac-live-scenarios.js \
//     -e QUEUE_URL=http://127.0.0.1:18101 -e USAGE_URL=http://127.0.0.1:18102 \
//     -e BUILD_URL=http://127.0.0.1:18103 -e K8S_URL=http://127.0.0.1:18104 \
//     -e NEXUSPAAS_PERF_API_KEY=<admin key>
import exec from 'k6/execution';
import http from 'k6/http';
import { check, sleep } from 'k6';

const apiKey = __ENV.NEXUSPAAS_PERF_API_KEY || '';
const vus = Number.parseInt(__ENV.NEXUSPAAS_PERF_VUS || '30', 10);
const duration = __ENV.NEXUSPAAS_PERF_DURATION || '60s';
const summaryPath = __ENV.NEXUSPAAS_PERF_SUMMARY_PATH || '.tmp/nexuspaas-perf-ac-live-scenarios.json';

// 4xx are expected business answers (quota rejections, validation); only 5xx
// and transport errors count as failures.
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 499 }));

const params = (scenario) => ({
  headers: { 'X-API-Key': apiKey, 'Content-Type': 'application/json' },
  timeout: '10s',
  tags: { scenario },
});

export const options = {
  scenarios: {
    queue_stress: { // PERF-003
      executor: 'constant-vus', exec: 'queueStress', vus, duration, gracefulStop: '5s',
    },
    usage_query: { // PERF-004
      executor: 'constant-vus', exec: 'usageQuery', vus, duration, gracefulStop: '5s',
    },
    build_load: { // PERF-006
      executor: 'constant-vus', exec: 'buildLoad', vus, duration, gracefulStop: '5s',
    },
    k8s_control: { // PERF-008
      executor: 'constant-vus', exec: 'k8sControl', vus, duration, gracefulStop: '5s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{scenario:queue_stress}': ['p(95)<1000'],
    'http_req_duration{scenario:usage_query}': ['p(95)<500'],
    'http_req_duration{scenario:build_load}': ['p(95)<1000'],
    'http_req_duration{scenario:k8s_control}': ['p(95)<500'],
    'http_reqs{scenario:queue_stress}': ['count>0'],
    'http_reqs{scenario:usage_query}': ['count>0'],
    'http_reqs{scenario:build_load}': ['count>0'],
    'http_reqs{scenario:k8s_control}': ['count>0'],
  },
};

export function queueStress() {
  const base = __ENV.QUEUE_URL;
  const id = `perf003-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const submit = http.post(`${base}/api/v1/jobs`, JSON.stringify({
    job_id: id,
    project_id: 'drift-drill-project',
    user_id: 'kind-admin',
    queue_name: 'default-batch',
    required_cpu: 1,
    required_memory: 512,
    resources: { cpu: 1, memory_mb: 512 },
  }), params('queue_stress'));
  check(submit, { 'queue submit answered without 5xx': (r) => r.status > 0 && r.status < 500 });
  const list = http.get(`${base}/api/v1/jobs`, params('queue_stress'));
  check(list, { 'queue list 200': (r) => r.status === 200 });
  sleep(0.2);
}

export function usageQuery() {
  const base = __ENV.USAGE_URL;
  for (const path of ['/api/v1/me/usage', '/api/v1/cluster/summary']) {
    const res = http.get(`${base}${path}`, params('usage_query'));
    check(res, { [`usage ${path} 200`]: (r) => r.status === 200 });
  }
  sleep(0.2);
}

export function buildLoad() {
  const base = __ENV.BUILD_URL;
  const id = `perf006-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const res = http.post(`${base}/api/v1/images/build/dockerfile`, JSON.stringify({
    id,
    project_id: 'drift-drill-project',
    image_reference: `registry.local/perf/app:${id}`,
    cpu_cores: 1,
    memory_gib: 1,
    dockerfile: 'FROM scratch\n',
  }), params('build_load'));
  check(res, { 'build create answered without 5xx': (r) => r.status > 0 && r.status < 500 });
  sleep(0.2);
}

export function k8sControl() {
  const base = __ENV.K8S_URL;
  for (const path of ['/api/v1/k8s/cluster', '/api/v1/k8s/nodes']) {
    const res = http.get(`${base}${path}`, params('k8s_control'));
    check(res, { [`k8s ${path} answered without 5xx`]: (r) => r.status > 0 && r.status < 500 });
  }
  sleep(0.2);
}

export function handleSummary(data) {
  return { [summaryPath]: JSON.stringify(data, null, 2), stdout: textSummary(data) };
}

function textSummary(data) {
  const lines = ['', 'PERF ac-live-scenarios summary:'];
  for (const [name, metric] of Object.entries(data.metrics)) {
    if (name === 'http_req_duration' && metric.values) {
      lines.push(`  http_req_duration p95=${metric.values['p(95)'].toFixed(1)}ms avg=${metric.values.avg.toFixed(1)}ms`);
    }
    if (name === 'http_req_failed' && metric.values) {
      lines.push(`  http_req_failed rate=${(metric.values.rate * 100).toFixed(3)}%`);
    }
    if (name === 'http_reqs' && metric.values) {
      lines.push(`  http_reqs count=${metric.values.count} rps=${metric.values.rate.toFixed(1)}`);
    }
  }
  lines.push('');
  return lines.join('\n');
}

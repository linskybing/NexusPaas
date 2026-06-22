import type {
  ConfigFilePayload,
  ConfigFileRecord,
  DashboardData,
  HealthStatus,
  JobLogRecord,
  JobRecord,
  JobSubmitPayload,
  OpenAPISummary,
  OutboxEvent,
  ProjectionStatus,
  ProjectGPUUsage,
  ProjectImageBuildRecord,
  ProjectImageRecord,
  ProjectRecord,
  ServiceSpec,
  StreamCredentialRequest,
  StreamCredentials,
  UsageRecord,
  WorkloadData,
} from "./types";

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: { message?: string; code?: string };
};

export class APIError extends Error {
  constructor(
    message: string,
    readonly status?: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}

export type APIClientConfig = {
  baseURL: string;
  apiKey: string;
  timeoutMs?: number;
};

export type APIClient = ReturnType<typeof createAPIClient>;

export function normalizeBaseURL(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

export function createAPIClient(config: APIClientConfig) {
  const baseURL = normalizeBaseURL(config.baseURL);
  const timeoutMs = config.timeoutMs ?? 10_000;

  async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
    return requestJSON<T>({ baseURL, path, apiKey: config.apiKey, timeoutMs, signal });
  }

  async function post<T>(path: string, body: unknown, signal?: AbortSignal, headers?: Record<string, string>): Promise<T> {
    return requestJSON<T>({ baseURL, path, apiKey: config.apiKey, timeoutMs, signal, method: "POST", body, headers });
  }

  return {
    health: (signal?: AbortSignal) => get<HealthStatus>("/healthz", signal),
    ready: (signal?: AbortSignal) => get<HealthStatus>("/readyz", signal),
    serviceRegistry: (signal?: AbortSignal) => get<ServiceSpec[]>("/service-registry", signal),
    outbox: (signal?: AbortSignal) => get<OutboxEvent[]>("/outbox", signal),
    projections: (signal?: AbortSignal) => get<ProjectionStatus[]>("/projections", signal),
    projects: (signal?: AbortSignal) => get<ProjectRecord[]>("/api/v1/projects", signal),
    configFiles: (projectID?: string, signal?: AbortSignal) =>
      get<ConfigFileRecord[]>(projectID ? `/api/v1/projects/${encodeURIComponent(projectID)}/config-files` : "/api/v1/configfiles", signal),
    jobs: (signal?: AbortSignal) => get<JobRecord[]>("/api/v1/jobs", signal),
    jobLogs: (id: string, signal?: AbortSignal) => get<JobLogRecord[]>(`/api/v1/jobs/${encodeURIComponent(id)}/logs`, signal),
    projectImages: (projectID: string, signal?: AbortSignal) =>
      get<ProjectImageRecord[]>(`/api/v1/projects/${encodeURIComponent(projectID)}/images`, signal),
    projectImageBuilds: (projectID: string, signal?: AbortSignal) =>
      get<ProjectImageBuildRecord[]>(`/api/v1/projects/${encodeURIComponent(projectID)}/image-builds`, signal),
    myUsage: (signal?: AbortSignal) => get<UsageRecord[]>("/api/v1/me/usage", signal),
    myRequestUsage: (signal?: AbortSignal) => get<UsageRecord[]>("/api/v1/me/request-usage", signal),
    projectGPUUsage: (projectID: string, signal?: AbortSignal) =>
      get<ProjectGPUUsage>(`/api/v1/projects/${encodeURIComponent(projectID)}/gpu-usage`, signal),
    workloads: async (projectID?: string, signal?: AbortSignal): Promise<WorkloadData> => {
      const [configFiles, jobs] = await Promise.all([
        get<ConfigFileRecord[]>(projectID ? `/api/v1/projects/${encodeURIComponent(projectID)}/config-files` : "/api/v1/configfiles", signal),
        get<JobRecord[]>("/api/v1/jobs", signal),
      ]);
      return { configFiles, jobs, projectScoped: Boolean(projectID) };
    },
    submitConfigFile: (payload: ConfigFilePayload, signal?: AbortSignal) =>
      post<ConfigFileRecord>("/api/v1/configfiles", payload, signal),
    submitJob: (payload: JobSubmitPayload, signal?: AbortSignal) =>
      post<JobRecord>(
        "/api/v1/jobs",
        payload,
        signal,
        { "Idempotency-Key": `nexuspaas-ui-submit-${payload.job_id}-${Date.now()}` },
      ),
    cancelJob: (id: string, signal?: AbortSignal) =>
      post<JobRecord>(
        `/api/v1/jobs/${encodeURIComponent(id)}/cancel`,
        {},
        signal,
        { "Idempotency-Key": `nexuspaas-ui-cancel-${id}-${Date.now()}` },
      ),
    streamCredentials: (payload: StreamCredentialRequest, signal?: AbortSignal) =>
      post<StreamCredentials>("/api/v1/stream/credentials", payload, signal),
    openapi: async (signal?: AbortSignal) => summarizeOpenAPI(await get<unknown>("/openapi.json", signal)),
    dashboard: async (signal?: AbortSignal): Promise<DashboardData> => {
      const [health, ready, services, outbox, projections, openapi, projectResult] = await Promise.all([
        get<HealthStatus>("/healthz", signal),
        get<HealthStatus>("/readyz", signal),
        get<ServiceSpec[]>("/service-registry", signal),
        get<OutboxEvent[]>("/outbox", signal),
        get<ProjectionStatus[]>("/projections", signal),
        get<unknown>("/openapi.json", signal).then(summarizeOpenAPI),
        get<ProjectRecord[]>("/api/v1/projects", signal)
          .then((projects) => ({ projects, projectsUnavailable: false }))
          .catch(() => ({ projects: [], projectsUnavailable: true })),
      ]);
      return { health, ready, services, outbox, projections, openapi, ...projectResult };
    },
  };
}

export async function requestJSON<T>(options: {
  baseURL: string;
  path: string;
  apiKey: string;
  timeoutMs: number;
  signal?: AbortSignal;
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  headers?: Record<string, string>;
}): Promise<T> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), options.timeoutMs);
  const signal = anySignal([controller.signal, options.signal]);
  const headers: Record<string, string> = {
    Accept: "application/json",
    ...(options.apiKey ? { "X-API-Key": options.apiKey } : {}),
    ...options.headers,
  };
  const init: RequestInit = {
    method: options.method ?? "GET",
    headers,
    credentials: "same-origin",
    signal,
  };
  if (options.body !== undefined) {
    init.body = JSON.stringify(options.body);
    if (!headers["Content-Type"]) {
      headers["Content-Type"] = "application/json";
    }
  }

  try {
    const response = await fetch(options.baseURL + options.path, init);
    const text = await response.text();
    const body = (text ? JSON.parse(text) : undefined) as Envelope<T> | T | undefined;
    if (!response.ok) {
      const envelope = body as Envelope<T>;
      throw new APIError(envelope.error?.message || response.statusText, response.status);
    }
    if (body === undefined) {
      return undefined as T;
    }
    if (isEnvelope<T>(body)) {
      if (body.success === false) {
        throw new APIError(body.error?.message || "Request failed", response.status);
      }
      return body.data as T;
    }
    return body as T;
  } finally {
    window.clearTimeout(timeout);
  }
}

function isEnvelope<T>(value: Envelope<T> | T): value is Envelope<T> {
  return typeof value === "object" && value !== null && ("success" in value || "data" in value || "error" in value);
}

function anySignal(signals: Array<AbortSignal | undefined>): AbortSignal {
  const controller = new AbortController();
  for (const signal of signals) {
    if (!signal) {
      continue;
    }
    if (signal.aborted) {
      controller.abort();
      break;
    }
    signal.addEventListener("abort", () => controller.abort(), { once: true });
  }
  return controller.signal;
}

export function summarizeOpenAPI(value: unknown): OpenAPISummary {
  const doc = value as {
    info?: { title?: string; version?: string };
    paths?: Record<string, Record<string, unknown>>;
  };
  const paths = doc.paths ?? {};
  const pathCount = Object.keys(paths).length;
  const operationCount = Object.values(paths).reduce((total, methods) => total + Object.keys(methods ?? {}).length, 0);
  return {
    title: doc.info?.title ?? "OpenAPI",
    version: doc.info?.version ?? "unknown",
    pathCount,
    operationCount,
  };
}

export type HealthStatus = {
  status: string;
  reason?: string;
};

export type ServiceRoute = {
  method: string;
  pattern: string;
  operation_id: string;
  resource: string;
  action: string;
  auth_required: boolean;
  admin: boolean;
  state_changing: boolean;
};

export type ServiceSpec = {
  name: string;
  category: string;
  phase: string;
  description: string;
  routes: ServiceRoute[];
  events: string[];
  tables: string[];
  requires_cluster?: boolean;
};

export type OutboxEvent = {
  event_id: string;
  name: string;
  source: string;
  occurred_at: string;
  trace_id: string;
  schema_version: number;
  idempotency_key?: string;
  data?: Record<string, unknown>;
};

export type ProjectionStatus = {
  consumer?: string;
  name?: string;
  lag?: number;
  dead_letters?: number;
  last_error?: string;
  [key: string]: unknown;
};

export type OpenAPISummary = {
  title: string;
  version: string;
  pathCount: number;
  operationCount: number;
};

export type ProjectRecord = {
  id?: string;
  project_id?: string;
  p_id?: string;
  PID?: string;
  name?: string;
  project_name?: string;
  ProjectName?: string;
  owner_id?: string;
  g_id?: string;
  GID?: string;
  [key: string]: unknown;
};

export type APIRecord<T extends Record<string, unknown>> = {
  id: string;
  data: T;
  version?: number;
  created_at?: string;
  updated_at?: string;
};

export type ConfigFileData = {
  id?: string;
  name?: string;
  path?: string;
  project_id?: string;
  projectId?: string;
  content?: string;
  updated_at?: string;
  [key: string]: unknown;
};

export type ConfigFileRecord = APIRecord<ConfigFileData>;

export type ConfigFilePayload = {
  project_id: string;
  name: string;
  path?: string;
  content: string;
};

export type JobData = {
  id?: string;
  job_id?: string;
  jobId?: string;
  project_id?: string;
  projectId?: string;
  user_id?: string;
  userId?: string;
  status?: string;
  submitted_at?: string;
  updated_at?: string;
  [key: string]: unknown;
};

export type JobRecord = APIRecord<JobData>;

export type JobLogData = {
  id?: string;
  job_id?: string;
  jobId?: string;
  line?: string;
  message?: string;
  text?: string;
  content?: string;
  timestamp?: string;
  time?: string;
  [key: string]: unknown;
};

export type JobLogRecord = APIRecord<JobLogData>;

export type JobSubmitPayload = {
  job_id: string;
  project_id: string;
  user_id: string;
  queue_name: string;
  required_cpu: number;
  required_memory: number;
  streaming_session?: boolean;
  stream_max_bitrate_kbps?: number;
};

export type WorkloadData = {
  configFiles: ConfigFileRecord[];
  jobs: JobRecord[];
  projectScoped: boolean;
};

export type StreamCredentialRequest = {
  job_id: string;
  session_id?: string;
  ttl_seconds?: number;
};

export type StreamTURNCredentials = {
  uris?: string[];
  username?: string;
  password?: string;
  ttl_seconds?: number;
  expires_at?: string;
};

export type StreamCredentials = {
  job_id?: string;
  turn?: StreamTURNCredentials;
};

export type ProjectImageRecord = {
  id?: string;
  tag_id?: string;
  tagId?: string;
  repository?: string;
  tag?: string;
  digest?: string;
  image_reference?: string;
  imageReference?: string;
  status?: string;
  scan_status?: string;
  scanStatus?: string;
  deleted?: boolean;
  unavailable?: boolean;
  [key: string]: unknown;
};

export type ProjectImageBuildRecord = {
  id?: string;
  build_id?: string;
  buildId?: string;
  job_name?: string;
  jobName?: string;
  image_reference?: string;
  imageReference?: string;
  status?: string;
  build_type?: string;
  buildType?: string;
  [key: string]: unknown;
};

export type UsageRecord = {
  UserID?: string;
  Username?: string;
  ProjectID?: string;
  ProjectName?: string;
  JobID?: string;
  CPUHours?: number;
  GPUHours?: number;
  MemoryGBHours?: number;
  PeriodStart?: string;
  PeriodEnd?: string;
  IsFinalized?: boolean;
  LastComputedAt?: string;
  user_id?: string;
  username?: string;
  project_id?: string;
  project_name?: string;
  job_id?: string;
  cpu_hours?: number;
  gpu_hours?: number;
  memory_gb_hours?: number;
  period_start?: string;
  period_end?: string;
  is_finalized?: boolean;
  last_computed_at?: string;
  [key: string]: unknown;
};

export type ProjectGPUUsage = {
  used?: number;
  Used?: number;
  [key: string]: unknown;
};

export type DashboardData = {
  health: HealthStatus;
  ready: HealthStatus;
  services: ServiceSpec[];
  outbox: OutboxEvent[];
  projections: ProjectionStatus[];
  openapi: OpenAPISummary;
  projects: ProjectRecord[];
  projectsUnavailable?: boolean;
};

import {
  Activity,
  Database,
  FileJson,
  KeyRound,
  ListChecks,
  LogIn,
  LogOut,
  RefreshCw,
  Route,
  Send,
  Server,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { createAPIClient } from "./api";
import { useDashboardData } from "./useDashboardData";
import type {
  APIRecord,
  ConfigFilePayload,
  ConfigFileRecord,
  DashboardData,
  ImageBuildPayload,
  JobData,
  JobLogData,
  JobLogRecord,
  JobRecord,
  JobSubmitPayload,
  OutboxEvent,
  ProjectionStatus,
  ProjectGPUUsage,
  ProjectImageBuildRecord,
  ProjectImageRecord,
  ProjectRecord,
  ServiceSpec,
  StreamCredentials,
  UsageRecord,
  WorkloadData,
} from "./types";

const defaultAPIBase = import.meta.env.VITE_API_BASE_URL || "";

export default function App() {
  const [baseURL, setBaseURL] = useState(defaultAPIBase);
  const [apiKey, setAPIKey] = useState("");
  const [apiKeyInput, setAPIKeyInput] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const cookieAuth = oidcCookieAuthMode();
  const authEnabled = Boolean(apiKey) || cookieAuth;
  const dashboard = useDashboardData(baseURL, apiKey, refreshKey, cookieAuth);

  function connect(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setAPIKey(apiKeyInput);
    setAPIKeyInput("");
    setRefreshKey((value) => value + 1);
  }

  function disconnect() {
    setAPIKey("");
    setAPIKeyInput("");
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">NexusPaaS</p>
          <h1>Operations</h1>
        </div>
        <div className="topbar-actions">
          <button className="icon-button" type="button" onClick={() => setRefreshKey((value) => value + 1)} aria-label="Refresh">
            <RefreshCw size={18} />
          </button>
          <a className="button secondary" href="/api/v1/oidc/start" aria-label="Sign in with OIDC">
            <LogIn size={16} />
            Sign in with OIDC
          </a>
          <button className="button secondary" type="button" onClick={disconnect}>
            <LogOut size={16} />
            Disconnect
          </button>
        </div>
      </header>

      <section className="connection-panel" aria-label="Connection">
        <form onSubmit={connect}>
          <label>
            API base URL
            <input
              value={baseURL}
              onChange={(event) => setBaseURL(event.target.value)}
              placeholder="http://127.0.0.1:18080"
              spellCheck={false}
            />
          </label>
          <label>
            Admin API key
            <input
              aria-label="Admin API key"
              value={apiKeyInput}
              onChange={(event) => setAPIKeyInput(event.target.value)}
              type="password"
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <button className="button primary" type="submit">
            <KeyRound size={16} />
            Connect
          </button>
        </form>
      </section>

      {dashboard.error ? <div className="error-banner">{dashboard.error}</div> : null}

      {dashboard.data ? (
        <Dashboard
          data={dashboard.data}
          loading={dashboard.loading}
          baseURL={baseURL}
          apiKey={apiKey}
          authEnabled={authEnabled}
          refreshKey={refreshKey}
        />
      ) : (
        <section className="empty-state" aria-label="No connection">
          <ShieldCheck size={22} />
          <span>{authEnabled ? "Loading..." : "Disconnected"}</span>
        </section>
      )}
    </main>
  );
}

function oidcCookieAuthMode(): boolean {
  return new URLSearchParams(window.location.search).get("auth") === "oidc";
}

function Dashboard({
  data,
  loading,
  baseURL,
  apiKey,
  authEnabled,
  refreshKey,
}: {
  data: DashboardData;
  loading: boolean;
  baseURL: string;
  apiKey: string;
  authEnabled: boolean;
  refreshKey: number;
}) {
  const [activeProjectID, setActiveProjectID] = useState("");
  const activeProject = data.projects.find((project) => projectID(project) === activeProjectID) ?? data.projects[0];
  const activeID = projectID(activeProject);

  return (
    <>
      <section className="status-grid" aria-label="Runtime status">
        <StatusTile icon={<Activity size={20} />} label="Health" value={data.health.status} tone={data.health.status} />
        <StatusTile icon={<ShieldCheck size={20} />} label="Readiness" value={data.ready.status} tone={data.ready.status} />
        <StatusTile icon={<Server size={20} />} label="Services" value={String(data.services.length)} />
        <StatusTile icon={<Route size={20} />} label="Routes" value={String(routeTotal(data.services))} />
        <StatusTile icon={<Database size={20} />} label="Outbox" value={String(data.outbox.length)} />
        <StatusTile icon={<FileJson size={20} />} label="OpenAPI" value={`${data.openapi.pathCount}/${data.openapi.operationCount}`} />
      </section>

      {loading ? <div className="sync-line">Refreshing</div> : null}

      <section className="content-grid">
        <ProjectSelector
          projects={data.projects}
          activeID={activeID}
          unavailable={data.projectsUnavailable}
          onChange={setActiveProjectID}
        />
        <WorkloadsPanel
          baseURL={baseURL}
          apiKey={apiKey}
          authEnabled={authEnabled}
          activeProjectID={activeID}
          unavailable={data.projectsUnavailable}
          refreshKey={refreshKey}
        />
        <ImagesPanel
          baseURL={baseURL}
          apiKey={apiKey}
          authEnabled={authEnabled}
          activeProjectID={activeID}
          unavailable={data.projectsUnavailable}
          refreshKey={refreshKey}
        />
        <UsagePanel
          baseURL={baseURL}
          apiKey={apiKey}
          authEnabled={authEnabled}
          activeProjectID={activeID}
          unavailable={data.projectsUnavailable}
          refreshKey={refreshKey}
        />
        <ServiceRegistry services={data.services} />
        <Outbox events={data.outbox} />
        <Projections projections={data.projections} />
        <OpenAPIPanel title={data.openapi.title} version={data.openapi.version} />
      </section>
    </>
  );
}

function ProjectSelector({
  projects,
  activeID,
  unavailable,
  onChange,
}: {
  projects: ProjectRecord[];
  activeID: string;
  unavailable?: boolean;
  onChange: (id: string) => void;
}) {
  const active = projects.find((project) => projectID(project) === activeID);
  return (
    <section className="panel">
      <PanelHeader icon={<ShieldCheck size={18} />} title="Projects" />
      <div className="project-picker">
        {unavailable ? (
          <p>Projects unavailable</p>
        ) : projects.length === 0 ? (
          <p>No project access</p>
        ) : (
          <>
            <label>
              Active project
              <select aria-label="Active project" value={activeID} onChange={(event) => onChange(event.target.value)}>
                {projects.map((project) => (
                  <option key={projectID(project)} value={projectID(project)}>
                    {projectName(project)}
                  </option>
                ))}
              </select>
            </label>
            <dl className="details compact">
              <div>
                <dt>ID</dt>
                <dd>{activeID}</dd>
              </div>
              <div>
                <dt>Owner</dt>
                <dd>{projectOwner(active)}</dd>
              </div>
            </dl>
          </>
        )}
      </div>
    </section>
  );
}

type WorkloadState = {
  data?: WorkloadData;
  loading: boolean;
  error?: string;
  message?: string;
};

type LogState = {
  jobID?: string;
  projectID?: string;
  logs: JobLogRecord[];
  loading: boolean;
  refreshing?: boolean;
  tailing?: boolean;
  lastRefreshedAt?: string;
  error?: string;
};

type StreamState = {
  jobID?: string;
  credentials?: StreamCredentials;
  passwordIssued?: boolean;
  loading: boolean;
  error?: string;
};

const jobLogPollMs = 5_000;

type ImageState = {
  images: ProjectImageRecord[];
  builds: ProjectImageBuildRecord[];
  loading: boolean;
  error?: string;
};

type UsageState = {
  usage: UsageRecord[];
  requestUsage: UsageRecord[];
  projectGPUUsage?: ProjectGPUUsage;
  projectGPUError?: string;
  loading: boolean;
  error?: string;
};

function WorkloadsPanel({
  baseURL,
  apiKey,
  authEnabled,
  activeProjectID,
  unavailable,
  refreshKey,
}: {
  baseURL: string;
  apiKey: string;
  authEnabled: boolean;
  activeProjectID: string;
  unavailable?: boolean;
  refreshKey: number;
}) {
  const client = useMemo(() => createAPIClient({ baseURL, apiKey }), [baseURL, apiKey]);
  const [state, setState] = useState<WorkloadState>({ loading: false });
  const [logState, setLogState] = useState<LogState>({ logs: [], loading: false });
  const [streamState, setStreamState] = useState<StreamState>({ loading: false });
  const [localRefreshKey, setLocalRefreshKey] = useState(0);
  const [busyJobID, setBusyJobID] = useState("");
  const [form, setForm] = useState({ name: "", path: "", content: "" });
  const [jobForm, setJobForm] = useState({
    jobID: "",
    userID: "e2e-admin",
    queueName: "default-batch",
    requiredCPU: "1",
    requiredMemory: "1024",
    streaming: false,
    streamMaxBitrate: "12000",
  });
  const disabled = unavailable || !activeProjectID;

  useEffect(() => {
    setLogState({ logs: [], loading: false });
    setStreamState({ loading: false });
  }, [activeProjectID]);

  useEffect(() => {
    const selectedJobID = logState.jobID;
    if (!selectedJobID || logState.projectID !== activeProjectID || !logState.tailing) {
      return;
    }
    if (disabled || !authEnabled) {
      setLogState({ logs: [], loading: false });
      return;
    }

    let stopped = false;
    let inFlight = false;
    let controller: AbortController | undefined;

    const refresh = async () => {
      if (inFlight) {
        return;
      }
      inFlight = true;
      const requestController = new AbortController();
      controller = requestController;
      setLogState((current) =>
        current.jobID === selectedJobID
          ? { ...current, loading: current.logs.length === 0, refreshing: current.logs.length > 0, error: undefined }
          : current,
      );
      try {
        const [logsResult, dataResult] = await Promise.allSettled([
          client.jobLogs(selectedJobID, requestController.signal),
          client.workloads(activeProjectID, requestController.signal),
        ]);
        if (stopped || requestController.signal.aborted) {
          return;
        }
        if (dataResult.status === "fulfilled") {
          const data = dataResult.value;
          setState((current) => ({ data, loading: false, message: current.message }));
          if (!filterJobsForProject(data.jobs, activeProjectID).some((job) => jobID(job) === selectedJobID)) {
            setLogState((current) =>
              current.jobID === selectedJobID
                ? {
                    jobID: selectedJobID,
                    projectID: activeProjectID,
                    logs: logsResult.status === "fulfilled" ? logsResult.value : current.logs,
                    loading: false,
                    refreshing: false,
                    tailing: false,
                    error: "Selected job is no longer available",
                  }
                : current,
            );
            return;
          }
        }
        if (logsResult.status === "fulfilled") {
          setLogState((current) =>
            current.jobID === selectedJobID
              ? {
                  jobID: selectedJobID,
                  projectID: activeProjectID,
                  logs: logsResult.value,
                  loading: false,
                  refreshing: false,
                  tailing: true,
                  lastRefreshedAt: new Date().toISOString(),
                  error: dataResult.status === "rejected" ? "Log/status refresh failed" : undefined,
                }
              : current,
          );
          return;
        }
        setLogState((current) =>
          current.jobID === selectedJobID
            ? { ...current, loading: false, refreshing: false, tailing: true, error: "Log/status refresh failed" }
            : current,
        );
      } catch {
        if (!stopped && !requestController.signal.aborted) {
          setLogState((current) =>
            current.jobID === selectedJobID
              ? { ...current, loading: false, refreshing: false, tailing: true, error: "Log/status refresh failed" }
              : current,
          );
        }
      } finally {
        inFlight = false;
        controller = undefined;
      }
    };

    void refresh();
    const timer = window.setInterval(refresh, jobLogPollMs);
    return () => {
      stopped = true;
      window.clearInterval(timer);
      controller?.abort();
    };
  }, [activeProjectID, authEnabled, client, disabled, logState.jobID, logState.projectID, logState.tailing]);

  useEffect(() => {
    if (disabled || !authEnabled) {
      setState({ loading: false });
      return;
    }

    const controller = new AbortController();
    setState((current) => ({ data: current.data, loading: true, message: current.message }));

    client
      .workloads(activeProjectID, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) {
          setState((current) => ({ data, loading: false, message: current.message }));
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState((current) => ({
            data: current.data,
            loading: false,
            error: error instanceof Error ? error.message : "Workload request failed",
          }));
        }
      });

    return () => controller.abort();
  }, [activeProjectID, authEnabled, client, disabled, localRefreshKey, refreshKey]);

  function updateForm(field: keyof typeof form, value: string) {
    setForm((current) => ({ ...current, [field]: value }));
  }

  function updateJobForm(field: keyof typeof jobForm, value: string | boolean) {
    setJobForm((current) => ({ ...current, [field]: value }));
  }

  async function submitConfigFile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (disabled || !form.name.trim() || !form.content.trim()) {
      return;
    }

    const payload: ConfigFilePayload = {
      project_id: activeProjectID,
      name: form.name.trim(),
      path: form.path.trim() || undefined,
      content: form.content,
    };

    setState((current) => ({ ...current, loading: true, error: undefined, message: undefined }));
    try {
      await client.submitConfigFile(payload);
      setForm({ name: "", path: "", content: "" });
      setState((current) => ({ ...current, loading: false, message: "ConfigFile submitted" }));
      setLocalRefreshKey((value) => value + 1);
    } catch (error) {
      setState((current) => ({
        ...current,
        loading: false,
        error: error instanceof Error ? error.message : "ConfigFile submit failed",
      }));
    }
  }

  async function submitJob(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const requiredCPU = Number(jobForm.requiredCPU);
    const requiredMemory = Number(jobForm.requiredMemory);
    const streamMaxBitrate = Number(jobForm.streamMaxBitrate);
    if (
      disabled ||
      !jobForm.jobID.trim() ||
      !jobForm.userID.trim() ||
      !jobForm.queueName.trim() ||
      !Number.isFinite(requiredCPU) ||
      requiredCPU <= 0 ||
      !Number.isFinite(requiredMemory) ||
      requiredMemory <= 0 ||
      (jobForm.streaming && (!Number.isFinite(streamMaxBitrate) || streamMaxBitrate <= 0))
    ) {
      return;
    }

    const payload: JobSubmitPayload = {
      job_id: jobForm.jobID.trim(),
      project_id: activeProjectID,
      user_id: jobForm.userID.trim(),
      queue_name: jobForm.queueName.trim(),
      required_cpu: requiredCPU,
      required_memory: requiredMemory,
      ...(jobForm.streaming ? { streaming_session: true, stream_max_bitrate_kbps: streamMaxBitrate } : {}),
    };

    setState((current) => ({ ...current, loading: true, error: undefined, message: undefined }));
    try {
      await client.submitJob(payload);
      setJobForm((current) => ({ ...current, jobID: "" }));
      setState((current) => ({ ...current, loading: false, message: "Job submitted" }));
      setLocalRefreshKey((value) => value + 1);
    } catch (error) {
      setState((current) => ({
        ...current,
        loading: false,
        error: error instanceof Error ? error.message : "Job submit failed",
      }));
    }
  }

  async function cancelJob(job: JobRecord) {
    const id = jobID(job);
    if (!id || busyJobID) {
      return;
    }
    setBusyJobID(id);
    setState((current) => ({ ...current, error: undefined, message: undefined }));
    try {
      await client.cancelJob(id);
      setState((current) => ({ ...current, message: "Cancel requested" }));
      setLocalRefreshKey((value) => value + 1);
    } catch (error) {
      setState((current) => ({
        ...current,
        error: error instanceof Error ? error.message : "Job cancel failed",
      }));
    } finally {
      setBusyJobID("");
    }
  }

  async function viewJobLogs(job: JobRecord) {
    const id = jobID(job);
    if (!id) {
      return;
    }
    setLogState({ jobID: id, projectID: activeProjectID, logs: [], loading: true, tailing: true });
  }

  async function openStream(job: JobRecord) {
    const id = jobID(job);
    if (!id || streamState.loading) {
      return;
    }
    setStreamState({ jobID: id, loading: true });
    try {
      const credentials = await client.streamCredentials({
        job_id: id,
        session_id: streamSessionID(id),
        ttl_seconds: 300,
      });
      setStreamState({
        jobID: id,
        credentials: redactStreamCredentials(credentials),
        passwordIssued: Boolean(credentials.turn?.password),
        loading: false,
      });
    } catch (error) {
      setStreamState({
        jobID: id,
        loading: false,
        error: error instanceof Error ? error.message : "Stream credential request failed",
      });
    }
  }

  const configFiles = state.data?.configFiles ?? [];
  const jobs = useMemo(
    () => filterJobsForProject(state.data?.jobs ?? [], activeProjectID),
    [activeProjectID, state.data?.jobs],
  );
  const canSubmit = Boolean(activeProjectID && form.name.trim() && form.content.trim() && !state.loading);
  const canSubmitJob = Boolean(
    activeProjectID &&
      jobForm.jobID.trim() &&
      jobForm.userID.trim() &&
      jobForm.queueName.trim() &&
      Number(jobForm.requiredCPU) > 0 &&
      Number(jobForm.requiredMemory) > 0 &&
      (!jobForm.streaming || Number(jobForm.streamMaxBitrate) > 0) &&
      !state.loading,
  );

  return (
    <section className="panel workloads-panel">
      <PanelHeader icon={<ListChecks size={18} />} title="Workloads" />
      {disabled ? (
        <div className="panel-body muted">{unavailable ? "Workloads unavailable" : "No active project"}</div>
      ) : (
        <div className="workloads-body">
          <div className="workload-meta">
            <span className="mono">{activeProjectID}</span>
            {state.loading ? <span>Refreshing</span> : null}
            {state.message ? <span className="success-text">{state.message}</span> : null}
          </div>
          {state.error ? <div className="inline-error" role="alert">{state.error}</div> : null}
          <form className="config-form" aria-label="Submit ConfigFile" onSubmit={submitConfigFile}>
            <label>
              Name
              <input
                value={form.name}
                onChange={(event) => updateForm("name", event.target.value)}
                placeholder="train.yaml"
                spellCheck={false}
              />
            </label>
            <label>
              Path
              <input
                value={form.path}
                onChange={(event) => updateForm("path", event.target.value)}
                placeholder="configs/train.yaml"
                spellCheck={false}
              />
            </label>
            <label className="span-2">
              Content
              <textarea
                value={form.content}
                onChange={(event) => updateForm("content", event.target.value)}
                spellCheck={false}
              />
            </label>
            <button className="button primary" type="submit" disabled={!canSubmit}>
              <Send size={16} />
              Submit ConfigFile
            </button>
          </form>
          <form className="config-form" aria-label="Submit Job" onSubmit={submitJob}>
            <label>
              Job ID
              <input
                value={jobForm.jobID}
                onChange={(event) => updateJobForm("jobID", event.target.value)}
                placeholder="train-001"
                spellCheck={false}
              />
            </label>
            <label>
              User ID
              <input
                value={jobForm.userID}
                onChange={(event) => updateJobForm("userID", event.target.value)}
                spellCheck={false}
              />
            </label>
            <label>
              Queue
              <input
                value={jobForm.queueName}
                onChange={(event) => updateJobForm("queueName", event.target.value)}
                spellCheck={false}
              />
            </label>
            <label>
              CPU
              <input
                type="number"
                min="0.1"
                step="0.1"
                value={jobForm.requiredCPU}
                onChange={(event) => updateJobForm("requiredCPU", event.target.value)}
              />
            </label>
            <label>
              Memory MB
              <input
                type="number"
                min="1"
                step="1"
                value={jobForm.requiredMemory}
                onChange={(event) => updateJobForm("requiredMemory", event.target.value)}
              />
            </label>
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={jobForm.streaming}
                onChange={(event) => updateJobForm("streaming", event.target.checked)}
              />
              Streaming
            </label>
            <label>
              Stream bitrate Kbps
              <input
                type="number"
                min="1"
                step="1"
                value={jobForm.streamMaxBitrate}
                disabled={!jobForm.streaming}
                onChange={(event) => updateJobForm("streamMaxBitrate", event.target.value)}
              />
            </label>
            <button className="button primary" type="submit" disabled={!canSubmitJob}>
              <Send size={16} />
              Submit Job
            </button>
          </form>
          <div className="workload-tables">
            <ConfigFilesTable records={configFiles} />
            <JobsTable
              jobs={jobs}
              busyJobID={busyJobID}
              busyStreamJobID={streamState.loading ? streamState.jobID || "" : ""}
              onCancel={cancelJob}
              onViewLogs={viewJobLogs}
              onOpenStream={openStream}
            />
            <JobLogsTable state={logState} />
            <StreamSessionPanel state={streamState} />
          </div>
        </div>
      )}
    </section>
  );
}

function ImagesPanel({
  baseURL,
  apiKey,
  authEnabled,
  activeProjectID,
  unavailable,
  refreshKey,
}: {
  baseURL: string;
  apiKey: string;
  authEnabled: boolean;
  activeProjectID: string;
  unavailable?: boolean;
  refreshKey: number;
}) {
  const client = useMemo(() => createAPIClient({ baseURL, apiKey }), [baseURL, apiKey]);
  const [state, setState] = useState<ImageState>({ images: [], builds: [], loading: false });
  const [imageReference, setImageReference] = useState("");
  const [submitMessage, setSubmitMessage] = useState("");
  const [submitError, setSubmitError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [localRefreshKey, setLocalRefreshKey] = useState(0);
  const disabled = unavailable || !activeProjectID;

  useEffect(() => {
    if (disabled || !authEnabled) {
      setState({ images: [], builds: [], loading: false });
      return;
    }

    const controller = new AbortController();
    setState((current) => ({ ...current, loading: true, error: undefined }));

    Promise.all([client.projectImages(activeProjectID, controller.signal), client.projectImageBuilds(activeProjectID, controller.signal)])
      .then(([images, builds]) => {
        if (!controller.signal.aborted) {
          setState({ images, builds, loading: false });
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState((current) => ({
            ...current,
            loading: false,
            error: "Image refresh failed",
          }));
        }
      });

    return () => controller.abort();
  }, [activeProjectID, authEnabled, client, disabled, localRefreshKey, refreshKey]);

  const canSubmitImageBuild = Boolean(activeProjectID && authEnabled && imageReference.trim() && !submitting);

  async function submitImageBuild(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const reference = imageReference.trim();
    if (!canSubmitImageBuild || !reference) {
      return;
    }

    const payload: ImageBuildPayload = { project_id: activeProjectID, image_reference: reference };
    setSubmitting(true);
    setSubmitMessage("");
    setSubmitError("");
    try {
      const build = await client.submitDockerfileImageBuild(payload);
      setImageReference("");
      setSubmitMessage(imageBuildSubmitMessage(build));
      setLocalRefreshKey((value) => value + 1);
    } catch {
      setSubmitError("Image build request failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="panel images-panel">
      <PanelHeader icon={<FileJson size={18} />} title="Images" />
      {disabled ? (
        <div className="panel-body muted">{unavailable ? "Images unavailable" : "No active project"}</div>
      ) : (
        <div className="panel-stack">
          <div className="workload-meta">
            <span className="mono">{activeProjectID}</span>
            {state.loading ? <span>Refreshing</span> : null}
            {submitMessage ? <span className="success-text">{submitMessage}</span> : null}
          </div>
          <form className="config-form" aria-label="Submit image build" onSubmit={submitImageBuild}>
            <label>
              Image reference
              <input
                value={imageReference}
                onChange={(event) => setImageReference(event.target.value)}
                placeholder="registry.local/team/app:tag"
                spellCheck={false}
              />
            </label>
            <label>
              Build type
              <input value="Dockerfile" readOnly disabled />
            </label>
            <button className="button primary" type="submit" disabled={!canSubmitImageBuild}>
              <Send size={16} />
              Submit image build
            </button>
          </form>
          {state.error ? <div className="inline-error" role="alert">{state.error}</div> : null}
          {submitError ? <div className="inline-error" role="alert">{submitError}</div> : null}
          <ImageTable records={state.images} />
          <ImageBuildTable records={state.builds} />
        </div>
      )}
    </section>
  );
}

function UsagePanel({
  baseURL,
  apiKey,
  authEnabled,
  activeProjectID,
  unavailable,
  refreshKey,
}: {
  baseURL: string;
  apiKey: string;
  authEnabled: boolean;
  activeProjectID: string;
  unavailable?: boolean;
  refreshKey: number;
}) {
  const client = useMemo(() => createAPIClient({ baseURL, apiKey }), [baseURL, apiKey]);
  const [state, setState] = useState<UsageState>({ usage: [], requestUsage: [], loading: false });
  const [localRefreshKey, setLocalRefreshKey] = useState(0);
  const disabled = unavailable || !activeProjectID;

  useEffect(() => {
    if (disabled || !authEnabled) {
      setState({ usage: [], requestUsage: [], loading: false });
      return;
    }

    const controller = new AbortController();
    setState((current) => ({ ...current, loading: true, error: undefined, projectGPUError: undefined }));

    Promise.allSettled([
      client.myUsage(controller.signal),
      client.myRequestUsage(controller.signal),
      client.projectGPUUsage(activeProjectID, controller.signal),
    ]).then(([usage, requestUsage, projectGPUUsage]) => {
      if (controller.signal.aborted) {
        return;
      }
      const usageRows = usage.status === "fulfilled" ? usage.value : [];
      const requestRows = requestUsage.status === "fulfilled" ? requestUsage.value : [];
      const projectGPU = projectGPUUsage.status === "fulfilled" ? projectGPUUsage.value : undefined;
      const usageError = [usage, requestUsage].some((result) => result.status === "rejected");
      setState({
        usage: usageRows,
        requestUsage: requestRows,
        projectGPUUsage: projectGPU,
        projectGPUError: projectGPUUsage.status === "rejected" ? "GPU usage refresh failed" : undefined,
        loading: false,
        error: usageError ? "Usage refresh failed" : undefined,
      });
    });

    return () => controller.abort();
  }, [activeProjectID, authEnabled, client, disabled, localRefreshKey, refreshKey]);

  const usageRows = useMemo(() => filterUsageForProject(state.usage, activeProjectID), [activeProjectID, state.usage]);
  const requestRows = useMemo(() => filterUsageForProject(state.requestUsage, activeProjectID), [activeProjectID, state.requestUsage]);

  return (
    <section className="panel usage-panel">
      <PanelHeader icon={<Activity size={18} />} title="Usage" />
      {disabled ? (
        <div className="panel-body muted">{unavailable ? "Usage unavailable" : "No active project"}</div>
      ) : (
        <div className="panel-stack">
          <div className="workload-meta">
            <span className="mono">{activeProjectID}</span>
            {state.loading ? <span>Refreshing</span> : null}
            <button
              className="icon-button small"
              type="button"
              aria-label="Refresh usage"
              disabled={state.loading}
              onClick={() => setLocalRefreshKey((value) => value + 1)}
            >
              <RefreshCw size={15} />
            </button>
          </div>
          {state.error ? <div className="inline-error" role="alert">{state.error}</div> : null}
          <ProjectGPUUsageSummary usage={state.projectGPUUsage} error={state.projectGPUError} />
          <UsageTable title="GPU usage" records={usageRows} />
          <UsageTable title="Request usage" records={requestRows} />
        </div>
      )}
    </section>
  );
}

function ConfigFilesTable({ records }: { records: ConfigFileRecord[] }) {
  return (
    <div className="table-block">
      <h3>ConfigFiles</h3>
      <table aria-label="ConfigFiles">
        <thead>
          <tr>
            <th>Name</th>
            <th>Path</th>
            <th>Updated</th>
          </tr>
        </thead>
        <tbody>
          {records.length === 0 ? (
            <tr>
              <td colSpan={3}>No ConfigFiles</td>
            </tr>
          ) : (
            records.map((record) => (
              <tr key={configFileID(record)}>
                <td>{configFileName(record)}</td>
                <td>{text(record.data.path)}</td>
                <td>{shortTime(text(record.updated_at ?? record.data.updated_at))}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function JobsTable({
  jobs,
  busyJobID,
  busyStreamJobID,
  onCancel,
  onViewLogs,
  onOpenStream,
}: {
  jobs: JobRecord[];
  busyJobID: string;
  busyStreamJobID: string;
  onCancel: (job: JobRecord) => void;
  onViewLogs: (job: JobRecord) => void;
  onOpenStream: (job: JobRecord) => void;
}) {
  return (
    <div className="table-block">
      <h3>Jobs</h3>
      <table aria-label="Jobs">
        <thead>
          <tr>
            <th>Job</th>
            <th>Status</th>
            <th>Logs</th>
            <th>Stream</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          {jobs.length === 0 ? (
            <tr>
              <td colSpan={5}>No jobs</td>
            </tr>
          ) : (
            jobs.map((job) => {
              const id = jobID(job);
              const cancelable = isCancelableJob(job);
              const streaming = isStreamingJob(job);
              return (
                <tr key={id}>
                  <td>{id}</td>
                  <td>{jobStatus(job)}</td>
                  <td>
                    <button className="button small" type="button" aria-label={`View logs ${id}`} disabled={!id} onClick={() => onViewLogs(job)}>
                      <FileJson size={14} />
                      View logs
                    </button>
                  </td>
                  <td>
                    {streaming ? (
                      <button
                        className="button small"
                        type="button"
                        aria-label={`Open stream ${id}`}
                        disabled={!id || busyStreamJobID === id}
                        onClick={() => onOpenStream(job)}
                      >
                        <KeyRound size={14} />
                        Open stream
                      </button>
                    ) : (
                      <span className="muted">Unavailable</span>
                    )}
                  </td>
                  <td>
                    <button
                      className="icon-button small"
                      type="button"
                      aria-label={`Cancel ${id}`}
                      disabled={!cancelable || busyJobID === id}
                      onClick={() => onCancel(job)}
                    >
                      <XCircle size={15} />
                    </button>
                  </td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}

function StreamSessionPanel({ state }: { state: StreamState }) {
  if (!state.jobID) {
    return null;
  }
  const turn = state.credentials?.turn;
  const uris = turn?.uris ?? [];
  return (
    <div className="table-block">
      <h3>Stream session {state.jobID}</h3>
      {state.error ? <div className="inline-error" role="alert">{state.error}</div> : null}
      {state.loading ? <div className="panel-summary">Requesting stream credentials</div> : null}
      {turn ? (
        <dl className="details compact summary-details stream-details" aria-label={`Stream session ${state.jobID}`}>
          <div>
            <dt>TURN URIs</dt>
            <dd>{uris.length ? `${uris.length} (${uris[0]})` : "none"}</dd>
          </div>
          <div>
            <dt>Username</dt>
            <dd className="mono">{turn.username || "missing"}</dd>
          </div>
          <div>
            <dt>TTL</dt>
            <dd>{turn.ttl_seconds ? `${turn.ttl_seconds}s` : "unknown"}</dd>
          </div>
          <div>
            <dt>Expires</dt>
            <dd>{shortTime(turn.expires_at || "") || "unknown"}</dd>
          </div>
          <div>
            <dt>Password</dt>
            <dd>{state.passwordIssued ? "redacted" : "not issued"}</dd>
          </div>
        </dl>
      ) : null}
    </div>
  );
}

function JobLogsTable({ state }: { state: LogState }) {
  if (!state.jobID) {
    return null;
  }
  const status = state.refreshing || state.loading ? "Refreshing logs" : state.tailing ? "Tailing logs" : "";
  return (
    <div className="table-block">
      <h3>Logs for {state.jobID}</h3>
      {status ? (
        <div className="panel-summary">
          {status}
          {state.lastRefreshedAt ? ` - Last refreshed ${shortTime(state.lastRefreshedAt)}` : ""}
        </div>
      ) : null}
      {state.error ? <div className="inline-error" role="alert">{state.error}</div> : null}
      <table aria-label={`Job logs ${state.jobID}`}>
        <thead>
          <tr>
            <th>Time</th>
            <th>Line</th>
          </tr>
        </thead>
        <tbody>
          {state.loading ? (
            <tr>
              <td colSpan={2}>Loading logs</td>
            </tr>
          ) : state.logs.length === 0 ? (
            <tr>
              <td colSpan={2}>No logs</td>
            </tr>
          ) : (
            state.logs.map((record) => (
              <tr key={jobLogID(record)}>
                <td>{jobLogTime(record) || "current"}</td>
                <td>{jobLogLine(record)}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function ImageTable({ records }: { records: ProjectImageRecord[] }) {
  return (
    <div className="table-block">
      <h3>Project images</h3>
      <table aria-label="Project images">
        <thead>
          <tr>
            <th>Image</th>
            <th>Digest</th>
            <th>Scan</th>
            <th>State</th>
          </tr>
        </thead>
        <tbody>
          {records.length === 0 ? (
            <tr>
              <td colSpan={4}>No Project images</td>
            </tr>
          ) : (
            records.map((record) => (
              <tr key={imageKey(record)}>
                <td>{imageName(record)}</td>
                <td className="mono">{shortID(imageDigest(record))}</td>
                <td>{imageScan(record)}</td>
                <td>{imageState(record)}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function ImageBuildTable({ records }: { records: ProjectImageBuildRecord[] }) {
  return (
    <div className="table-block">
      <h3>Image builds</h3>
      <table aria-label="Image builds">
        <thead>
          <tr>
            <th>Build</th>
            <th>Image</th>
            <th>Type</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {records.length === 0 ? (
            <tr>
              <td colSpan={4}>No image builds</td>
            </tr>
          ) : (
            records.map((record) => (
              <tr key={imageBuildID(record)}>
                <td>{imageBuildID(record)}</td>
                <td>{imageBuildImage(record)}</td>
                <td>{valueText(record.build_type, record.buildType) || "unknown"}</td>
                <td>{valueText(record.status) || "unknown"}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function ProjectGPUUsageSummary({ usage, error }: { usage?: ProjectGPUUsage; error?: string }) {
  if (error) {
    return <div className="panel-summary muted">Project GPU usage unavailable: {error}</div>;
  }
  return (
    <dl className="details compact summary-details">
      <div>
        <dt>Project GPU pods</dt>
        <dd>{String(projectGPUUsed(usage))}</dd>
      </div>
    </dl>
  );
}

function UsageTable({ title, records }: { title: string; records: UsageRecord[] }) {
  const cpuHours = usageTotal(records, "CPUHours", "cpu_hours");
  const gpuHours = usageTotal(records, "GPUHours", "gpu_hours");
  const memoryGBHours = usageTotal(records, "MemoryGBHours", "memory_gb_hours");
  return (
    <div className="table-block">
      <h3>{title}</h3>
      <dl className="details compact summary-details" aria-label={`${title} totals`}>
        <div>
          <dt>Rows</dt>
          <dd>{records.length}</dd>
        </div>
        <div>
          <dt>CPUh</dt>
          <dd>{formatNumber(cpuHours)}</dd>
        </div>
        <div>
          <dt>GPUh</dt>
          <dd>{formatNumber(gpuHours)}</dd>
        </div>
        <div>
          <dt>Memory GBh</dt>
          <dd>{formatNumber(memoryGBHours)}</dd>
        </div>
      </dl>
      <table aria-label={title}>
        <thead>
          <tr>
            <th>Job</th>
            <th>Project</th>
            <th>CPUh</th>
            <th>GPUh</th>
            <th>Memory GBh</th>
            <th>Period</th>
          </tr>
        </thead>
        <tbody>
          {records.length === 0 ? (
            <tr>
              <td colSpan={6}>No {title}</td>
            </tr>
          ) : (
            records.map((record, index) => (
              <tr key={`${usageJobID(record)}-${index}`}>
                <td>{usageJobID(record) || "none"}</td>
                <td>{usageProjectName(record)}</td>
                <td>{formatNumber(usageNumber(record, "CPUHours", "cpu_hours"))}</td>
                <td>{formatNumber(usageNumber(record, "GPUHours", "gpu_hours"))}</td>
                <td>{formatNumber(usageNumber(record, "MemoryGBHours", "memory_gb_hours"))}</td>
                <td>{usagePeriod(record)}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function StatusTile({
  icon,
  label,
  value,
  tone,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  tone?: string;
}) {
  return (
    <article className="status-tile">
      <div className="tile-icon">{icon}</div>
      <div>
        <p>{label}</p>
        <strong className={tone === "ok" ? "ok" : ""}>{value}</strong>
      </div>
    </article>
  );
}

function ServiceRegistry({ services }: { services: ServiceSpec[] }) {
  return (
    <section className="panel wide">
      <PanelHeader icon={<Server size={18} />} title="Services" />
      <table aria-label="Services">
        <thead>
          <tr>
            <th>Name</th>
            <th>Category</th>
            <th>Phase</th>
            <th>Routes</th>
            <th>Events</th>
          </tr>
        </thead>
        <tbody>
          {services.map((service) => (
            <tr key={service.name}>
              <td>{service.name}</td>
              <td>{service.category}</td>
              <td>{service.phase}</td>
              <td>{service.routes?.length ?? 0}</td>
              <td>{service.events?.length ?? 0}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function Outbox({ events }: { events: OutboxEvent[] }) {
  const recent = useMemo(() => events.slice(-8).reverse(), [events]);
  return (
    <section className="panel">
      <PanelHeader icon={<Database size={18} />} title="Outbox" />
      <table aria-label="Outbox events">
        <thead>
          <tr>
            <th>Event</th>
            <th>Source</th>
            <th>Trace</th>
          </tr>
        </thead>
        <tbody>
          {recent.map((event) => (
            <tr key={event.event_id}>
              <td>{event.name}</td>
              <td>{event.source}</td>
              <td className="mono">{shortID(event.trace_id)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function Projections({ projections }: { projections: ProjectionStatus[] }) {
  return (
    <section className="panel">
      <PanelHeader icon={<Activity size={18} />} title="Projections" />
      <table aria-label="Projections">
        <thead>
          <tr>
            <th>Consumer</th>
            <th>Lag</th>
            <th>Dead letters</th>
          </tr>
        </thead>
        <tbody>
          {projections.length === 0 ? (
            <tr>
              <td colSpan={3}>No projection state</td>
            </tr>
          ) : (
            projections.map((projection, index) => (
              <tr key={`${projection.consumer ?? projection.name ?? "projection"}-${index}`}>
                <td>{String(projection.consumer ?? projection.name ?? "projection")}</td>
                <td>{String(projection.lag ?? 0)}</td>
                <td>{String(projection.dead_letters ?? 0)}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </section>
  );
}

function OpenAPIPanel({ title, version }: { title: string; version: string }) {
  return (
    <section className="panel">
      <PanelHeader icon={<FileJson size={18} />} title="OpenAPI" />
      <dl className="details">
        <div>
          <dt>Title</dt>
          <dd>{title}</dd>
        </div>
        <div>
          <dt>Version</dt>
          <dd>{version}</dd>
        </div>
      </dl>
    </section>
  );
}

function PanelHeader({ icon, title }: { icon: React.ReactNode; title: string }) {
  return (
    <header className="panel-header">
      {icon}
      <h2>{title}</h2>
    </header>
  );
}

function routeTotal(services: ServiceSpec[]): number {
  return services.reduce((total, service) => total + (service.routes?.length ?? 0), 0);
}

function shortID(value: string): string {
  return value ? value.slice(0, 12) : "";
}

function filterJobsForProject(jobs: JobRecord[], activeProjectID: string): JobRecord[] {
  if (!activeProjectID) {
    return [];
  }
  return jobs.filter((job) => jobProjectID(job) === activeProjectID);
}

function configFileID(record: ConfigFileRecord): string {
  return record.id || text(record.data.id);
}

function configFileName(record: ConfigFileRecord): string {
  return text(record.data.name) || text(record.data.path) || configFileID(record);
}

function jobID(record: JobRecord): string {
  return record.id || text(record.data.job_id ?? record.data.jobId ?? record.data.id);
}

function jobProjectID(record: APIRecord<JobData>): string {
  return text(record.data.project_id ?? record.data.projectId);
}

function jobStatus(record: APIRecord<JobData>): string {
  return text(record.data.status) || "unknown";
}

function jobLogID(record: APIRecord<JobLogData>): string {
  return record.id || text(record.data.id);
}

function jobLogTime(record: APIRecord<JobLogData>): string {
  return shortTime(text(record.data.timestamp ?? record.data.time ?? record.created_at ?? record.updated_at));
}

function jobLogLine(record: APIRecord<JobLogData>): string {
  return valueText(record.data.line, record.data.message, record.data.text, record.data.content, jobLogID(record));
}

function isCancelableJob(record: JobRecord): boolean {
  const status = jobStatus(record).toLowerCase();
  return Boolean(jobID(record)) && !["succeeded", "failed", "cancelled", "canceled"].includes(status);
}

function isStreamingJob(record: JobRecord): boolean {
  return boolValue(record.data.streaming_session ?? record.data.streamingSession ?? record.data.StreamingSession);
}

function streamSessionID(jobIDValue: string): string {
  return `ui-${streamSessionNamePart(jobIDValue)}-${Date.now()}`;
}

function streamSessionNamePart(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return "stream";
  }
  return trimmed.replace(/[^A-Za-z0-9_.-]+/g, "-").replace(/^-+|-+$/g, "") || "stream";
}

function redactStreamCredentials(credentials: StreamCredentials): StreamCredentials {
  if (!credentials.turn) {
    return credentials;
  }
  const { password: _password, ...turn } = credentials.turn;
  return { ...credentials, turn };
}

function filterUsageForProject(records: UsageRecord[], activeProjectID: string): UsageRecord[] {
  if (!activeProjectID) {
    return [];
  }
  return records.filter((record) => usageProjectID(record) === activeProjectID);
}

function imageKey(record: ProjectImageRecord): string {
  return valueText(record.id, record.tag_id, record.tagId, record.digest, record.image_reference, record.imageReference, imageName(record));
}

function imageName(record: ProjectImageRecord): string {
  const reference = valueText(record.image_reference, record.imageReference);
  if (reference) {
    return reference;
  }
  const repository = valueText(record.repository);
  const tag = valueText(record.tag);
  if (repository && tag) {
    return `${repository}:${tag}`;
  }
  return valueText(record.id, record.tag_id, record.tagId) || "image";
}

function imageDigest(record: ProjectImageRecord): string {
  return valueText(record.digest);
}

function imageScan(record: ProjectImageRecord): string {
  return valueText(record.scan_status, record.scanStatus) || "unknown";
}

function imageState(record: ProjectImageRecord): string {
  if (boolValue(record.deleted)) {
    return "deleted";
  }
  if (boolValue(record.unavailable)) {
    return "unavailable";
  }
  return valueText(record.status) || "available";
}

function imageBuildID(record: ProjectImageBuildRecord): string {
  return valueText(record.id, record.build_id, record.buildId, record.job_name, record.jobName) || "build";
}

function imageBuildImage(record: ProjectImageBuildRecord): string {
  return valueText(record.image_reference, record.imageReference) || imageBuildID(record);
}

function imageBuildSubmitMessage(record?: ProjectImageBuildRecord): string {
  const id = record ? valueText(record.id, record.build_id, record.buildId, record.job_name, record.jobName) : "";
  const status = record ? valueText(record.status) : "";
  const details = [id, status].filter(Boolean).join(" ");
  return details ? `Image build submitted: ${details}` : "Image build submitted";
}

function usageProjectID(record: UsageRecord): string {
  return valueText(record.ProjectID, record.project_id);
}

function usageProjectName(record: UsageRecord): string {
  return valueText(record.ProjectName, record.project_name) || usageProjectID(record) || "none";
}

function usageJobID(record: UsageRecord): string {
  return valueText(record.JobID, record.job_id);
}

function usageNumber(record: UsageRecord, pascalKey: keyof UsageRecord, snakeKey: keyof UsageRecord): number {
  return numberValue(record[pascalKey] ?? record[snakeKey]);
}

function usageTotal(records: UsageRecord[], pascalKey: keyof UsageRecord, snakeKey: keyof UsageRecord): number {
  return records.reduce((total, record) => total + usageNumber(record, pascalKey, snakeKey), 0);
}

function usagePeriod(record: UsageRecord): string {
  const start = shortTime(valueText(record.PeriodStart, record.period_start));
  const end = shortTime(valueText(record.PeriodEnd, record.period_end));
  return [start, end].filter(Boolean).join(" - ") || "current";
}

function projectGPUUsed(usage?: ProjectGPUUsage): number {
  return numberValue(usage?.used ?? usage?.Used);
}

function formatNumber(value: number): string {
  return Number.isFinite(value) ? value.toLocaleString(undefined, { maximumFractionDigits: 2 }) : "0";
}

function numberValue(value: unknown): number {
  if (typeof value === "number") {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function boolValue(value: unknown): boolean {
  return value === true || value === "true";
}

function valueText(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value;
    }
    if (typeof value === "number" || typeof value === "boolean") {
      return String(value);
    }
  }
  return "";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Request failed";
}

function shortTime(value: string): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value.slice(0, 16);
  }
  return date.toISOString().slice(0, 16).replace("T", " ");
}

function projectID(project?: ProjectRecord): string {
  return text(project?.id ?? project?.project_id ?? project?.p_id ?? project?.PID);
}

function projectName(project: ProjectRecord): string {
  return text(project.name ?? project.project_name ?? project.ProjectName) || projectID(project);
}

function projectOwner(project?: ProjectRecord): string {
  return text(project?.owner_id ?? project?.g_id ?? project?.GID) || "none";
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

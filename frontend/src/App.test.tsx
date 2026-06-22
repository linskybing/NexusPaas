import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import App from "./App";

describe("App", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    window.history.pushState({}, "", "/");
  });

  it("renders the connection form with a masked admin key field", () => {
    render(<App />);

    expect(screen.getByRole("heading", { name: "Operations" })).toBeInTheDocument();
    expect(screen.getByLabelText("Admin API key")).toHaveAttribute("type", "password");
    expect(screen.getByRole("button", { name: /^Connect$/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^Sign in with OIDC$/ })).toHaveAttribute("href", "/api/v1/oidc/start");
  });

  it("loads dashboard data from same-origin endpoints when API base is empty", async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ "X-API-Key": "adminkey" });
      switch (url) {
        case "/healthz":
        case "/readyz":
          return json({ success: true, data: { status: "ok" } });
        case "/service-registry":
          return json({ success: true, data: [{ name: "platform-gateway", category: "edge", phase: "1", routes: [] }] });
        case "/outbox":
        case "/projections":
          return json({ success: true, data: [] });
        case "/api/v1/projects":
          return json({
            success: true,
            data: [
              { id: "proj-a", project_name: "Project A", owner_id: "group-a" },
              { id: "proj-b", project_name: "Project B", owner_id: "group-b" },
            ],
          });
        case "/api/v1/projects/proj-a/config-files":
        case "/api/v1/projects/proj-b/config-files":
        case "/api/v1/jobs":
        case "/api/v1/projects/proj-a/images":
        case "/api/v1/projects/proj-a/image-builds":
        case "/api/v1/projects/proj-b/images":
        case "/api/v1/projects/proj-b/image-builds":
        case "/api/v1/me/usage":
        case "/api/v1/me/request-usage":
          return json({ success: true, data: [] });
        case "/api/v1/projects/proj-a/gpu-usage":
        case "/api/v1/projects/proj-b/gpu-usage":
          return json({ success: true, data: { used: 0 } });
        case "/openapi.json":
          return json({ info: { title: "NexusPaaS", version: "1" }, paths: { "/healthz": { get: {} } } });
        default:
          throw new Error(`unexpected fetch ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<App />);
    expect(screen.getByLabelText("API base URL")).toHaveValue("");

    fireEvent.change(screen.getByLabelText("Admin API key"), { target: { value: "adminkey" } });
    fireEvent.click(screen.getByRole("button", { name: /^Connect$/ }));

    await waitFor(() => expect(screen.getByText("platform-gateway")).toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "Projects" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Workloads" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Images" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Usage" })).toBeInTheDocument();
    expect(screen.getByLabelText("Active project")).toHaveValue("proj-a");
    fireEvent.change(screen.getByLabelText("Active project"), { target: { value: "proj-b" } });
    expect(screen.getByLabelText("Active project")).toHaveValue("proj-b");
    expect(screen.getByLabelText("Admin API key")).toHaveValue("");
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    expect(fetchMock).toHaveBeenCalledWith("/service-registry", expect.any(Object));
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/projects", expect.any(Object));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/v1/projects/proj-b/config-files", expect.any(Object)));
  });

  it("loads dashboard through OIDC cookie mode without an API key", async () => {
    window.history.pushState({}, "", "/ui/?auth=oidc");
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(init?.headers).not.toMatchObject({ "X-API-Key": expect.any(String) });
      switch (url) {
        case "/healthz":
        case "/readyz":
          return json({ success: true, data: { status: "ok" } });
        case "/service-registry":
          return json({ success: true, data: [{ name: "platform-gateway", category: "edge", phase: "1", routes: [] }] });
        case "/outbox":
        case "/projections":
        case "/api/v1/projects/proj-a/config-files":
        case "/api/v1/jobs":
        case "/api/v1/projects/proj-a/images":
        case "/api/v1/projects/proj-a/image-builds":
        case "/api/v1/me/usage":
        case "/api/v1/me/request-usage":
          return json({ success: true, data: [] });
        case "/api/v1/projects":
          return json({ success: true, data: [{ id: "proj-a", project_name: "Project A", owner_id: "group-a" }] });
        case "/api/v1/projects/proj-a/gpu-usage":
          return json({ success: true, data: { used: 0 } });
        case "/openapi.json":
          return json({ info: { title: "NexusPaaS", version: "1" }, paths: { "/healthz": { get: {} } } });
        default:
          throw new Error(`unexpected fetch ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<App />);

    await waitFor(() => expect(screen.getByText("platform-gateway")).toBeInTheDocument());
    expect(screen.getByLabelText("Admin API key")).toHaveValue("");
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    expect(fetchMock).toHaveBeenCalledWith("/service-registry", expect.objectContaining({ credentials: "same-origin" }));
  });

  it("keeps the dashboard available when the project API is unavailable", async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ "X-API-Key": "adminkey" });
      switch (url) {
        case "/healthz":
        case "/readyz":
          return json({ success: true, data: { status: "ok" } });
        case "/service-registry":
          return json({ success: true, data: [{ name: "platform-gateway", category: "edge", phase: "1", routes: [] }] });
        case "/outbox":
        case "/projections":
          return json({ success: true, data: [] });
        case "/api/v1/projects":
          return json({ success: false, error: { message: "Method Not Allowed" } }, 405);
        case "/openapi.json":
          return json({ info: { title: "NexusPaaS", version: "1" }, paths: { "/healthz": { get: {} } } });
        default:
          throw new Error(`unexpected fetch ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<App />);
    fireEvent.change(screen.getByLabelText("Admin API key"), { target: { value: "adminkey" } });
    fireEvent.click(screen.getByRole("button", { name: /^Connect$/ }));

    await waitFor(() => expect(screen.getByText("platform-gateway")).toBeInTheDocument());
    expect(screen.getByRole("heading", { name: "Projects" })).toBeInTheDocument();
    expect(screen.getByText("Projects unavailable")).toBeInTheDocument();
    expect(screen.queryByText("No project access")).not.toBeInTheDocument();
  });

  it("loads active-project workloads, submits ConfigFiles, and requests job cancel", async () => {
    let submittedJob = false;
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ "X-API-Key": "adminkey" });
      switch (`${init?.method ?? "GET"} ${url}`) {
        case "GET /healthz":
        case "GET /readyz":
          return json({ success: true, data: { status: "ok" } });
        case "GET /service-registry":
          return json({ success: true, data: [{ name: "workload-service", category: "compute", phase: "5", routes: [] }] });
        case "GET /outbox":
        case "GET /projections":
          return json({ success: true, data: [] });
        case "GET /api/v1/projects":
          return json({
            success: true,
            data: [
              { id: "proj-a", project_name: "Project A", owner_id: "group-a" },
              { id: "proj-b", project_name: "Project B", owner_id: "group-b" },
            ],
          });
        case "GET /api/v1/projects/proj-a/config-files":
          return json({
            success: true,
            data: [{ id: "cfg-a", data: { project_id: "proj-a", name: "train.yaml", path: "configs/train.yaml" } }],
          });
        case "GET /api/v1/jobs":
          return json({
            success: true,
            data: [
              { id: "job-a", data: { project_id: "proj-a", job_id: "job-a", status: "running", streaming_session: true } },
              ...(submittedJob ? [{ id: "job-new", data: { project_id: "proj-a", job_id: "job-new", status: "submitted" } }] : []),
              { id: "job-b", data: { project_id: "proj-b", job_id: "job-b", status: "running" } },
            ],
          });
	        case "GET /api/v1/projects/proj-a/images":
	          return json({
	            success: true,
	            data: [
	              {
	                tag_id: "tag-a",
	                repository: "registry.local/team/app",
	                tag: "stable",
	                digest: "sha256:abcdef",
	                scan_status: "passed",
	                status: "available",
	              },
	              {
	                tag_id: "tag-deleted",
	                image_reference: "registry.local/team/app:old",
	                digest: "sha256:123456",
	                scanStatus: "Failed",
	                deleted: true,
	              },
	              {
	                tag_id: "tag-unavailable",
	                repository: "registry.local/team/missing",
	                tag: "edge",
	                digest: "sha256:654321",
	                scan_status: "Pending",
	                unavailable: "true",
	              },
	            ],
	          });
        case "GET /api/v1/projects/proj-a/image-builds":
          return json({
            success: true,
            data: [{ id: "build-a", image_reference: "registry.local/team/app:stable", build_type: "dockerfile", status: "queued" }],
          });
        case "GET /api/v1/me/usage":
          return json({
            success: true,
            data: [
              { ProjectID: "proj-a", ProjectName: "Project A", JobID: "job-a", CPUHours: 1, GPUHours: 2, MemoryGBHours: 3 },
              { ProjectID: "proj-b", ProjectName: "Project B", JobID: "job-b", CPUHours: 4, GPUHours: 5, MemoryGBHours: 6 },
            ],
          });
        case "GET /api/v1/me/request-usage":
          return json({
            success: true,
            data: [{ ProjectID: "proj-a", ProjectName: "Project A", JobID: "job-a", CPUHours: 7, GPUHours: 8, MemoryGBHours: 9 }],
          });
        case "GET /api/v1/projects/proj-a/gpu-usage":
          return json({ success: true, data: { used: 2 } });
        case "POST /api/v1/configfiles":
          expect(init?.body).toBe(JSON.stringify({ project_id: "proj-a", name: "new.yaml", content: "kind: Job" }));
          return json({ success: true, data: { id: "cfg-new", data: { project_id: "proj-a", name: "new.yaml" } } }, 201);
        case "POST /api/v1/jobs":
          expect(init?.body).toBe(
            JSON.stringify({
              job_id: "job-new",
              project_id: "proj-a",
              user_id: "e2e-admin",
              queue_name: "queue-a",
              required_cpu: 1,
              required_memory: 1024,
            }),
          );
          submittedJob = true;
          return json({ success: true, data: { id: "job-new", data: { project_id: "proj-a", job_id: "job-new", status: "submitted" } } }, 201);
        case "POST /api/v1/jobs/job-new/cancel":
          return json({ success: true, data: { id: "cmd-a", data: { job_id: "job-new" } } }, 202);
        case "GET /api/v1/jobs/job-new/logs":
          return json({ success: true, data: [{ id: "log-a", data: { job_id: "job-new", line: "queued" } }] });
        case "POST /api/v1/stream/credentials": {
          const payload = JSON.parse(String(init?.body));
          expect(payload.job_id).toBe("job-a");
          expect(payload.session_id).toMatch(/^ui-job-a-\d+$/);
          expect(payload.ttl_seconds).toBe(300);
          return json({
            success: true,
            data: {
              job_id: "job-a",
              turn: {
                uris: ["turn:turn.example.com:3478?transport=udp"],
                username: "1893456000:e2e-admin",
                password: "turn-password-secret",
                ttl_seconds: 300,
                expires_at: "2030-01-01T00:00:00Z",
              },
            },
          });
        }
        case "GET /openapi.json":
          return json({ info: { title: "NexusPaaS", version: "1" }, paths: { "/api/v1/jobs": { get: {} } } });
        default:
          throw new Error(`unexpected fetch ${init?.method ?? "GET"} ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<App />);
    fireEvent.change(screen.getByLabelText("Admin API key"), { target: { value: "adminkey" } });
    fireEvent.click(screen.getByRole("button", { name: /^Connect$/ }));

    expect(await screen.findByText("train.yaml")).toBeInTheDocument();
	    expect(screen.getAllByText("job-a").length).toBeGreaterThan(0);
	    expect(screen.queryByText("job-b")).not.toBeInTheDocument();
	    expect((await screen.findAllByText("registry.local/team/app:stable")).length).toBeGreaterThan(0);
	    expect(screen.getByText("build-a")).toBeInTheDocument();
	    const imagesTable = screen.getByRole("table", { name: "Project images" });
	    expect(within(imagesTable).getByText("passed")).toBeInTheDocument();
	    expect(within(imagesTable).getByText("Failed")).toBeInTheDocument();
	    expect(within(imagesTable).getByText("Pending")).toBeInTheDocument();
	    expect(within(imagesTable).getByText("available")).toBeInTheDocument();
	    expect(within(imagesTable).getByText("deleted")).toBeInTheDocument();
	    expect(within(imagesTable).getByText("unavailable")).toBeInTheDocument();
	    expect(within(screen.getByRole("table", { name: "Image builds" })).getByText("queued")).toBeInTheDocument();
	    const projectGPUSummary = screen.getByText("Project GPU pods").closest("div");
	    expect(projectGPUSummary).not.toBeNull();
	    expect(within(projectGPUSummary as HTMLElement).getByText("2")).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "GPU usage" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open stream job-a" }));
    const streamDetails = await screen.findByLabelText("Stream session job-a");
    expect(streamDetails).toBeInTheDocument();
    expect(within(streamDetails).getByText("1 (turn:turn.example.com:3478?transport=udp)")).toBeInTheDocument();
    expect(within(streamDetails).getByText("1893456000:e2e-admin")).toBeInTheDocument();
    expect(within(streamDetails).getByText("redacted")).toBeInTheDocument();
    expect(screen.queryByText("turn-password-secret")).not.toBeInTheDocument();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "new.yaml" } });
    fireEvent.change(screen.getByLabelText("Content"), { target: { value: "kind: Job" } });
    fireEvent.click(screen.getByRole("button", { name: /Submit ConfigFile/ }));
    expect(await screen.findByText("ConfigFile submitted")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Job ID"), { target: { value: "job-new" } });
    fireEvent.change(screen.getByLabelText("Queue"), { target: { value: "queue-a" } });
    fireEvent.click(screen.getByRole("button", { name: /Submit Job/ }));
    expect(await screen.findByText("Job submitted")).toBeInTheDocument();
    expect(await screen.findByText("job-new")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "View logs job-new" }));
    const logsTable = await screen.findByRole("table", { name: "Job logs job-new" });
    expect(logsTable).toBeInTheDocument();
    expect(within(logsTable).getByText("queued")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cancel job-new" }));
    expect(await screen.findByText("Cancel requested")).toBeInTheDocument();
  });

  it("shows stream credential errors without losing the job list", async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ "X-API-Key": "adminkey" });
      switch (`${init?.method ?? "GET"} ${url}`) {
        case "GET /healthz":
        case "GET /readyz":
          return json({ success: true, data: { status: "ok" } });
        case "GET /service-registry":
          return json({ success: true, data: [{ name: "workload-service", category: "compute", phase: "5", routes: [] }] });
        case "GET /outbox":
        case "GET /projections":
        case "GET /api/v1/projects/proj-a/config-files":
        case "GET /api/v1/projects/proj-a/images":
        case "GET /api/v1/projects/proj-a/image-builds":
        case "GET /api/v1/me/usage":
        case "GET /api/v1/me/request-usage":
          return json({ success: true, data: [] });
        case "GET /api/v1/projects":
          return json({ success: true, data: [{ id: "proj-a", project_name: "Project A", owner_id: "group-a" }] });
        case "GET /api/v1/jobs":
          return json({ success: true, data: [{ id: "job-stream", data: { project_id: "proj-a", job_id: "job-stream", status: "running", streaming_session: true } }] });
        case "GET /api/v1/projects/proj-a/gpu-usage":
          return json({ success: true, data: { used: 0 } });
        case "POST /api/v1/stream/credentials":
          return json({ success: false, error: { message: "stream TURN credentials are not configured" } }, 503);
        case "GET /openapi.json":
          return json({ info: { title: "NexusPaaS", version: "1" }, paths: { "/api/v1/stream/credentials": { post: {} } } });
        default:
          throw new Error(`unexpected fetch ${init?.method ?? "GET"} ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<App />);
    fireEvent.change(screen.getByLabelText("Admin API key"), { target: { value: "adminkey" } });
    fireEvent.click(screen.getByRole("button", { name: /^Connect$/ }));

    expect(await screen.findByText("job-stream")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open stream job-stream" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("stream TURN credentials are not configured");
    expect(screen.getByText("job-stream")).toBeInTheDocument();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it("keeps image and usage panels available for empty or forbidden project data", async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ "X-API-Key": "adminkey" });
      switch (`${init?.method ?? "GET"} ${url}`) {
        case "GET /healthz":
        case "GET /readyz":
          return json({ success: true, data: { status: "ok" } });
        case "GET /service-registry":
          return json({ success: true, data: [{ name: "usage-observability-service", category: "ops-read-model", phase: "3", routes: [] }] });
        case "GET /outbox":
        case "GET /projections":
        case "GET /api/v1/projects/proj-a/config-files":
        case "GET /api/v1/jobs":
        case "GET /api/v1/projects/proj-a/images":
        case "GET /api/v1/projects/proj-a/image-builds":
        case "GET /api/v1/me/usage":
        case "GET /api/v1/me/request-usage":
          return json({ success: true, data: [] });
        case "GET /api/v1/projects":
          return json({ success: true, data: [{ id: "proj-a", project_name: "Project A", owner_id: "group-a" }] });
        case "GET /api/v1/projects/proj-a/gpu-usage":
          return json({ success: false, error: { message: "project access required" } }, 403);
        case "GET /openapi.json":
          return json({ info: { title: "NexusPaaS", version: "1" }, paths: { "/api/v1/projects/{id}/gpu-usage": { get: {} } } });
        default:
          throw new Error(`unexpected fetch ${init?.method ?? "GET"} ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<App />);
    fireEvent.change(screen.getByLabelText("Admin API key"), { target: { value: "adminkey" } });
    fireEvent.click(screen.getByRole("button", { name: /^Connect$/ }));

    expect(await screen.findByText("No Project images")).toBeInTheDocument();
    expect(screen.getByText("No image builds")).toBeInTheDocument();
    expect(await screen.findByText("Project GPU usage unavailable: project access required")).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalledWith("/api/v1/admin/usage", expect.any(Object));
  });
});

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

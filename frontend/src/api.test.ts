import { afterEach, describe, expect, it, vi } from "vitest";

import { APIError, createAPIClient, normalizeBaseURL, requestJSON, summarizeOpenAPI } from "./api";

describe("api client helpers", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("normalizes base URLs", () => {
    expect(normalizeBaseURL(" http://127.0.0.1:18080/// ")).toBe("http://127.0.0.1:18080");
    expect(normalizeBaseURL("   ")).toBe("");
  });

  it("unwraps backend envelopes and sends API keys", async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      expect(init?.headers).toMatchObject({ "X-API-Key": "adminkey" });
      return new Response(JSON.stringify({ success: true, data: { status: "ok" } }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      requestJSON<{ status: string }>({
        baseURL: "http://api",
        path: "/readyz",
        apiKey: "adminkey",
        timeoutMs: 500,
      }),
    ).resolves.toEqual({ status: "ok" });
  });

  it("uses same-origin paths when the base URL is empty", async () => {
    const fetchMock = vi.fn(async () => {
      return new Response(JSON.stringify({ success: true, data: { status: "ok" } }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await requestJSON<{ status: string }>({
      baseURL: "",
      path: "/healthz",
      apiKey: "adminkey",
      timeoutMs: 500,
    });

    expect(fetchMock).toHaveBeenCalledWith("/healthz", expect.objectContaining({ credentials: "same-origin" }));
    expect(fetchMock).toHaveBeenCalledWith("/healthz", expect.any(Object));
  });

  it("sends JSON bodies for state-changing requests", async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      expect(init?.method).toBe("POST");
      expect(init?.headers).toMatchObject({
        "Content-Type": "application/json",
        "X-API-Key": "adminkey",
      });
      expect(init?.body).toBe(JSON.stringify({ name: "train.yaml" }));
      return new Response(JSON.stringify({ success: true, data: { id: "cfg-1", data: { name: "train.yaml" } } }), { status: 201 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      requestJSON({
        baseURL: "",
        path: "/api/v1/configfiles",
        apiKey: "adminkey",
        timeoutMs: 500,
        method: "POST",
        body: { name: "train.yaml" },
      }),
    ).resolves.toEqual({ id: "cfg-1", data: { name: "train.yaml" } });
  });

  it("fetches projects through the existing project API", async () => {
    const fetchMock = vi.fn(async () => {
      return new Response(JSON.stringify({ success: true, data: [{ id: "P1", project_name: "Project One" }] }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(createAPIClient({ baseURL: "", apiKey: "adminkey" }).projects()).resolves.toEqual([
      { id: "P1", project_name: "Project One" },
    ]);
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/projects", expect.any(Object));
  });

  it("fetches workload and job log data through existing workload APIs", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      switch (url) {
        case "/api/v1/projects/P1/config-files":
          return new Response(JSON.stringify({ success: true, data: [{ id: "cfg-1", data: { project_id: "P1" } }] }), { status: 200 });
        case "/api/v1/jobs":
          return new Response(JSON.stringify({ success: true, data: [{ id: "job-1", data: { project_id: "P1" } }] }), { status: 200 });
        case "/api/v1/jobs/job-1/logs":
          return new Response(JSON.stringify({ success: true, data: [{ id: "log-1", data: { job_id: "job-1", line: "queued" } }] }), {
            status: 200,
          });
        default:
          throw new Error(`unexpected fetch ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(createAPIClient({ baseURL: "", apiKey: "adminkey" }).workloads("P1")).resolves.toEqual({
      configFiles: [{ id: "cfg-1", data: { project_id: "P1" } }],
      jobs: [{ id: "job-1", data: { project_id: "P1" } }],
      projectScoped: true,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/projects/P1/config-files", expect.any(Object));
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/jobs", expect.any(Object));
    await expect(createAPIClient({ baseURL: "", apiKey: "adminkey" }).jobLogs("job-1")).resolves.toEqual([
      { id: "log-1", data: { job_id: "job-1", line: "queued" } },
    ]);
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/jobs/job-1/logs", expect.any(Object));
  });

  it("submits ConfigFiles, submits jobs, and sends cancel requests", async () => {
    vi.spyOn(Date, "now").mockReturnValue(123);
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (url === "/api/v1/configfiles") {
        expect(init?.method).toBe("POST");
        expect(init?.body).toBe(JSON.stringify({ project_id: "P1", name: "train.yaml", content: "kind: Job" }));
        return new Response(JSON.stringify({ success: true, data: { id: "cfg-1", data: { name: "train.yaml" } } }), { status: 201 });
      }
      if (url === "/api/v1/jobs") {
        expect(init?.method).toBe("POST");
        expect(init?.headers).toMatchObject({ "Idempotency-Key": "nexuspaas-ui-submit-job-1-123" });
        expect(init?.body).toBe(
          JSON.stringify({ job_id: "job-1", project_id: "P1", user_id: "user-1", queue_name: "queue-a", required_cpu: 1, required_memory: 1024 }),
        );
        return new Response(JSON.stringify({ success: true, data: { id: "job-1", data: { project_id: "P1" } } }), { status: 201 });
      }
      if (url === "/api/v1/jobs/job-1/cancel") {
        expect(init?.method).toBe("POST");
        expect(init?.headers).toMatchObject({ "Idempotency-Key": "nexuspaas-ui-cancel-job-1-123" });
        return new Response(JSON.stringify({ success: true, data: { id: "cmd-1", data: { job_id: "job-1" } } }), { status: 202 });
      }
      throw new Error(`unexpected fetch ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = createAPIClient({ baseURL: "", apiKey: "adminkey" });

    await expect(client.submitConfigFile({ project_id: "P1", name: "train.yaml", content: "kind: Job" })).resolves.toEqual({
      id: "cfg-1",
      data: { name: "train.yaml" },
    });
    await expect(
      client.submitJob({ job_id: "job-1", project_id: "P1", user_id: "user-1", queue_name: "queue-a", required_cpu: 1, required_memory: 1024 }),
    ).resolves.toEqual({ id: "job-1", data: { project_id: "P1" } });
    await expect(client.cancelJob("job-1")).resolves.toEqual({ id: "cmd-1", data: { job_id: "job-1" } });
  });

  it("requests stream credentials through the existing workload API", async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      expect(url).toBe("/api/v1/stream/credentials");
      expect(init?.method).toBe("POST");
      expect(init?.headers).toMatchObject({
        "Content-Type": "application/json",
        "X-API-Key": "adminkey",
      });
      expect(init?.body).toBe(JSON.stringify({ job_id: "job-1", session_id: "ui-job-1", ttl_seconds: 300 }));
      return new Response(
        JSON.stringify({
          success: true,
          data: {
            job_id: "job-1",
            turn: {
              uris: ["turn:turn.example.com:3478?transport=udp"],
              username: "1893456000:alice",
              password: "secret-password",
              ttl_seconds: 300,
              expires_at: "2030-01-01T00:00:00Z",
            },
          },
        }),
        { status: 200 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createAPIClient({ baseURL: "", apiKey: "adminkey" }).streamCredentials({
        job_id: "job-1",
        session_id: "ui-job-1",
        ttl_seconds: 300,
      }),
    ).resolves.toEqual({
      job_id: "job-1",
      turn: {
        uris: ["turn:turn.example.com:3478?transport=udp"],
        username: "1893456000:alice",
        password: "secret-password",
        ttl_seconds: 300,
        expires_at: "2030-01-01T00:00:00Z",
      },
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/stream/credentials", expect.objectContaining({ credentials: "same-origin" }));
  });

  it("fetches Project image and usage views through existing APIs", async () => {
	    const fetchMock = vi.fn(async (url: string) => {
	      switch (url) {
	        case "/api/v1/projects/P1/images":
	          return new Response(
	            JSON.stringify({
	              success: true,
	              data: [
	                {
	                  tag_id: "tag-1",
	                  repository: "repo/app",
	                  digest: "sha256:abc",
	                  scan_status: "Success",
	                  deleted: false,
	                  unavailable: true,
	                  status: "unavailable",
	                },
	              ],
	            }),
	            { status: 200 },
	          );
        case "/api/v1/projects/P1/image-builds":
          return new Response(JSON.stringify({ success: true, data: [{ id: "build-1", status: "queued" }] }), { status: 200 });
        case "/api/v1/me/usage":
          return new Response(JSON.stringify({ success: true, data: [{ ProjectID: "P1", JobID: "J1", GPUHours: 2 }] }), { status: 200 });
        case "/api/v1/me/request-usage":
          return new Response(JSON.stringify({ success: true, data: [{ ProjectID: "P1", JobID: "J1", CPUHours: 1 }] }), { status: 200 });
        case "/api/v1/projects/P1/gpu-usage":
          return new Response(
            JSON.stringify({
              success: true,
              data: {
                used: 2,
                observed_gpu_pods: 2,
                reserved_gpu_fraction: 0.75,
                reserved_gpu_source: "cluster_read_model_allocation",
                sm_attribution_source: "estimated_mps_allocation",
              },
            }),
            { status: 200 },
          );
        default:
          throw new Error(`unexpected fetch ${url}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);
    const client = createAPIClient({ baseURL: "", apiKey: "adminkey" });

	    await expect(client.projectImages("P1")).resolves.toEqual([
	      {
	        tag_id: "tag-1",
	        repository: "repo/app",
	        digest: "sha256:abc",
	        scan_status: "Success",
	        deleted: false,
	        unavailable: true,
	        status: "unavailable",
	      },
	    ]);
    await expect(client.projectImageBuilds("P1")).resolves.toEqual([{ id: "build-1", status: "queued" }]);
    await expect(client.myUsage()).resolves.toEqual([{ ProjectID: "P1", JobID: "J1", GPUHours: 2 }]);
    await expect(client.myRequestUsage()).resolves.toEqual([{ ProjectID: "P1", JobID: "J1", CPUHours: 1 }]);
    await expect(client.projectGPUUsage("P1")).resolves.toEqual({
      used: 2,
      observed_gpu_pods: 2,
      reserved_gpu_fraction: 0.75,
      reserved_gpu_source: "cluster_read_model_allocation",
      sm_attribution_source: "estimated_mps_allocation",
    });
  });

  it("raises envelope errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        return new Response(JSON.stringify({ success: false, error: { message: "denied" } }), { status: 403 });
      }),
    );

    await expect(
      requestJSON({
        baseURL: "http://api",
        path: "/outbox",
        apiKey: "bad",
        timeoutMs: 500,
      }),
    ).rejects.toBeInstanceOf(APIError);
  });

  it("summarizes OpenAPI paths and operations", () => {
    expect(
      summarizeOpenAPI({
        info: { title: "NexusPaaS", version: "1" },
        paths: {
          "/a": { get: {}, post: {} },
          "/b": { get: {} },
        },
      }),
    ).toEqual({ title: "NexusPaaS", version: "1", pathCount: 2, operationCount: 3 });
  });
});

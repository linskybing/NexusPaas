import { expect, test, type APIRequestContext, type APIResponse, type Page } from "@playwright/test";

const apiKey = process.env.NEXUSPAAS_E2E_API_KEY;
const apiBaseURL = process.env.VITE_API_BASE_URL || "";
const appPath = process.env.NEXUSPAAS_E2E_APP_PATH || "/ui/";
const seedProject = process.env.NEXUSPAAS_E2E_SEED_PROJECT === "true";
const authMode = process.env.NEXUSPAAS_E2E_AUTH_MODE || "";
const isOIDCAuthMode = authMode === "oidc";
const oidcUsername = process.env.NEXUSPAAS_E2E_OIDC_USERNAME || "";
const oidcPassword = process.env.NEXUSPAAS_E2E_OIDC_PASSWORD || "";
const expectedProjectID = process.env.NEXUSPAAS_E2E_PROJECT_ID || "";
const expectedProjectName = process.env.NEXUSPAAS_E2E_PROJECT_NAME || "";
const expectedImageScanStatus = process.env.NEXUSPAAS_E2E_IMAGE_SCAN_STATUS || "";
const expectedImageState = process.env.NEXUSPAAS_E2E_IMAGE_STATE || "";
const seedStreamCredentials = process.env.NEXUSPAAS_E2E_STREAM_CREDENTIALS === "true";
const e2eEnvironment = process.env.NEXUSPAAS_E2E_ENVIRONMENT || "";
const rtcIceProbe = process.env.NEXUSPAAS_E2E_RTC_ICE_PROBE === "true";
const expectNonemptyLogs = process.env.NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS === "true";
const expectNonzeroGPU = process.env.NEXUSPAAS_E2E_EXPECT_NONZERO_GPU === "true";

type APIMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
type APIEnvelope<T> = {
  success?: boolean;
  data?: T;
  error?: { message?: string; code?: string };
};
type APIResult<T> = {
  ok: boolean;
  status: number;
  data?: T;
  error?: string;
};
type SeedState = {
  groupID: string;
  groupName: string;
  projectID: string;
  projectName: string;
  userID: string;
  queueID: string;
  planID: string;
  jobID: string;
  configName: string;
  groupCreated?: boolean;
  projectCreated?: boolean;
  queueCreated?: boolean;
  planCreated?: boolean;
  planBound?: boolean;
  configSubmitted?: boolean;
  configID?: string;
  jobSubmitted?: boolean;
  jobCancelRequested?: boolean;
  jobCancelCommandID?: string;
  jobLogsRequested?: boolean;
  jobLogsStatus?: number;
  jobLogsCount?: number;
  jobLogsNonempty?: boolean;
  jobLogsVisible?: boolean;
  streamCredentialsRequested?: boolean;
  streamCredentialsStatus?: number;
  streamCredentialURICount?: number;
  streamCredentialUsernamePresent?: boolean;
  streamCredentialPasswordIssued?: boolean;
  streamCredentialPasswordRedacted?: boolean;
  rtcProbeEnvironment?: string;
  rtcDirectOK?: boolean;
  rtcDirectCandidateCount?: number;
  rtcDirectCandidateTypes?: string[];
  rtcRelayOK?: boolean;
  rtcRelayCandidateCount?: number;
  rtcRelayCandidateTypes?: string[];
  gpuUsed?: number;
  gpuNonzero?: boolean;
  imageReference: string;
  imageRequestCreated?: boolean;
  imageRequestID?: string;
  imageBuildCreated?: boolean;
  imageRuleID?: string;
  imageBuildID?: string;
  workloadSeedStatus: string[];
  imageSeedStatus: string[];
  leftovers: string[];
};

type RTCProbeResult = {
  ok: boolean;
  completed: boolean;
  candidateCount: number;
  candidateTypes: string[];
  error?: string;
};

type RTCProbeSummary = {
  direct: RTCProbeResult;
  relay: RTCProbeResult;
};

test("renders live NexusPaaS operations dashboard", async ({ page, request }) => {
  test.skip(!isOIDCAuthMode && !apiKey, "NEXUSPAAS_E2E_API_KEY is required for live GUI smoke");
  test.skip(
    isOIDCAuthMode && (!oidcUsername || !oidcPassword),
    "NEXUSPAAS_E2E_OIDC_USERNAME and NEXUSPAAS_E2E_OIDC_PASSWORD are required when NEXUSPAAS_E2E_AUTH_MODE=oidc",
  );
  if (rtcIceProbe) {
    if (e2eEnvironment !== "staging") {
      throw new Error("NEXUSPAAS_E2E_ENVIRONMENT=staging is required for RTC ICE proof");
    }
    if (!seedProject || isOIDCAuthMode || !seedStreamCredentials) {
      throw new Error("RTC ICE proof requires API-key seeded Project E2E with NEXUSPAAS_E2E_STREAM_CREDENTIALS=true");
    }
  }
  if ((expectNonemptyLogs || expectNonzeroGPU) && (!seedProject || isOIDCAuthMode)) {
    throw new Error("log/GPU enforcement requires API-key seeded Project E2E");
  }

  const seed = seedProject && !isOIDCAuthMode ? newSeedState() : undefined;

  try {
    if (seed) {
      await seedLiveProject(request, seed);
    }

    await page.goto(appPath);

    await expect(page.getByRole("heading", { name: "Operations" })).toBeVisible();
    if (isOIDCAuthMode) {
      await authenticateWithOIDC(page);
    } else {
      await expect(page.getByLabel("API base URL")).toHaveValue("");
      if (apiBaseURL) {
        await page.getByLabel("API base URL").fill(apiBaseURL);
      }
      await page.getByLabel("Admin API key").fill(apiKey!);
      await page.getByRole("button", { name: "Connect", exact: true }).click();

      await expect(page.getByLabel("Admin API key")).toHaveValue("");
    }
    await assertDashboardPanelLayout(page);

    if (expectedProjectID) {
      await assertExpectedProjectImageStatusUI(page);
    }
    if (seed) {
      await assertSeededProjectUI(page, seed);
      await captureRouteProof(request, seed);
    }

    await page.screenshot({ path: seed ? "test-results/gui-live-seeded-project.png" : "test-results/gui-live-smoke.png", fullPage: true });
  } finally {
    if (seed) {
      await cleanupSeed(request, seed);
    }
  }
});

function newSeedState(): SeedState {
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
  return {
    groupID: `e2e-g-${suffix}`,
    groupName: `E2E Group ${suffix}`,
    projectID: `e2e-p-${suffix}`,
    projectName: `E2E Project ${suffix}`,
    userID: "e2e-admin",
    queueID: `e2e-q-${suffix}`,
    planID: `e2e-plan-${suffix}`,
    jobID: `e2e-job-${suffix}`,
    configName: `e2e-config-${suffix}.yaml`,
    imageReference: `nexuspaas-e2e:${suffix}`,
    workloadSeedStatus: [],
    imageSeedStatus: [],
    leftovers: [],
  };
}

async function authenticateWithOIDC(page: Page): Promise<void> {
  const signInLink = page.getByRole("link", { name: "Sign in with OIDC", exact: true });
  await expect(signInLink).toBeVisible();
  const signInURL = new URL((await signInLink.getAttribute("href"))!, page.url());
  expect(signInURL.pathname).toBe("/api/v1/oidc/start");

  await Promise.all([
    page.waitForLoadState("domcontentloaded"),
    signInLink.click(),
  ]);

  const usernameInput = page
    .locator('input[name="login"], input[name="username"], input[autocomplete="username"], input[type="text"]')
    .first();
  const passwordInput = page.locator('input[name="password"], input[autocomplete="current-password"], input[type="password"]').first();
  await expect(usernameInput).toBeVisible();
  await expect(passwordInput).toBeVisible();
  await usernameInput.fill(oidcUsername);
  await passwordInput.fill(oidcPassword);

  const submitButton = page
    .locator(
      'button[type="submit"], button:has-text("Sign In"), button:has-text("Sign in"), button:has-text("Log in"), button:has-text("Login"), button:has-text("Continue")',
    )
    .first();
  await expect(submitButton).toBeVisible();
  await Promise.all([
    page.waitForURL((url) => {
      const parsed = new URL(url);
      return parsed.pathname === "/ui/" && parsed.searchParams.get("auth") === "oidc";
    }),
    submitButton.click(),
  ]);

  const cookies = await page.context().cookies();
  const cookieNames = new Set(cookies.map((cookie) => cookie.name));
  expect(cookieNames.has("token")).toBeTruthy();
  expect(cookieNames.has("refresh_token")).toBeTruthy();

  const hasApiOrTokenStorageItem = await page.evaluate(() => {
    const buckets = [window.localStorage, window.sessionStorage];
    const sensitive = /api[-_]?key|access[_-]?token|refresh[_-]?token|id[_-]?token/i;
    for (const store of buckets) {
      for (let i = 0; i < store.length; i++) {
        const key = store.key(i);
        if (!key) continue;
        if (sensitive.test(key)) {
          return true;
        }
        const value = store.getItem(key) || "";
        if (/^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(value)) {
          return true;
        }
      }
    }
    return false;
  });
  expect(hasApiOrTokenStorageItem).toBeFalsy();
}

async function assertDashboardPanelLayout(page: Page): Promise<void> {
  await expect(page.getByRole("table", { name: "Services" })).toBeVisible();
  await expect(page.getByText("platform-gateway")).toBeVisible();
  await expect(page.getByRole("table", { name: "Outbox events" })).toBeVisible();
  await expect(page.getByRole("table", { name: "Projections" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Projects", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Workloads", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Images", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Usage", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "OpenAPI", exact: true })).toBeVisible();
}

async function seedLiveProject(request: APIRequestContext, seed: SeedState): Promise<void> {
  await apiJSON(request, "POST", "/api/v1/groups", {
    id: seed.groupID,
    group_name: seed.groupName,
    name: seed.groupName,
    description: "NexusPaaS live E2E seed group",
  });
  seed.groupCreated = true;
  await apiJSON(request, "POST", "/api/v1/projects", {
    id: seed.projectID,
    project_name: seed.projectName,
    name: seed.projectName,
    g_id: seed.groupID,
    group_id: seed.groupID,
    personal_user_id: seed.userID,
    description: "NexusPaaS live E2E seed project",
  });
  seed.projectCreated = true;
  console.log(`nexuspaas live seed created group=${seed.groupID} project=${seed.projectID}`);
  await seedSchedulerData(request, seed);
  await seedOptionalImageData(request, seed);
}

async function seedSchedulerData(request: APIRequestContext, seed: SeedState): Promise<void> {
  await apiJSON(request, "POST", "/api/v1/queues", {
    id: seed.queueID,
    name: seed.queueID,
    priority_value: 10,
    max_runtime_seconds: 3600,
  });
  seed.queueCreated = true;
  seed.workloadSeedStatus.push(`queue=${seed.queueID}`);
  await apiJSON(request, "POST", "/api/v1/plans", {
    id: seed.planID,
    name: seed.planID,
    queue_ids: [seed.queueID],
    queues: [seed.queueID],
    max_cpu: 8,
    max_memory: 8192,
    max_gpu: 0,
    max_jobs: 4,
  });
  seed.planCreated = true;
  seed.workloadSeedStatus.push(`plan=${seed.planID}`);
  await apiJSON(request, "PUT", `/api/v1/plans/bind/${encodeURIComponent(seed.projectID)}`, {
    plan_id: seed.planID,
  });
  seed.planBound = true;
  seed.workloadSeedStatus.push(`plan_bound=${seed.planID}`);
  console.log(`nexuspaas live workload seed ${JSON.stringify({ project_id: seed.projectID, status: seed.workloadSeedStatus })}`);
}

async function seedOptionalImageData(request: APIRequestContext, seed: SeedState): Promise<void> {
  seed.imageRequestID = `e2e-img-${seed.projectID}`;
  seed.imageBuildID = `e2e-build-${seed.projectID}`;
  try {
    await apiJSON(request, "POST", `/api/v1/projects/${encodeURIComponent(seed.projectID)}/images`, {
      id: seed.imageRequestID,
      image_reference: seed.imageReference,
    });
    seed.imageRequestCreated = true;
    seed.imageSeedStatus.push(`image_request=${seed.imageRequestID}`);
    await apiJSON(request, "PUT", `/api/v1/image-requests/${encodeURIComponent(seed.imageRequestID)}/approve`);
    seed.imageSeedStatus.push("image_request_approved");
    const build = await apiJSON(request, "POST", "/api/v1/images/build", {
      id: seed.imageBuildID,
      project_id: seed.projectID,
      image_reference: seed.imageReference,
    });
    if (!imageBuildMatchesSeed(build, seed)) {
      throw new Error("image build response did not include the seeded build id and project id");
    }
    seed.imageBuildCreated = true;
    seed.imageSeedStatus.push(`image_build=${seed.imageBuildID}`);
  } catch (error) {
    seed.imageSeedStatus.push(`optional_image_seed_rejected=${errorMessage(error)}`);
  }
  console.log(`nexuspaas live optional image seed ${JSON.stringify({ project_id: seed.projectID, status: seed.imageSeedStatus })}`);
}

async function assertSeededProjectUI(page: Page, seed: SeedState) {
  const projectSelect = page.getByLabel("Active project");
  await expect(projectSelect).toBeVisible();
  await expect(projectSelect).toContainText(seed.projectName);
  await projectSelect.selectOption(seed.projectID);
  await expect(projectSelect).toHaveValue(seed.projectID);

  await expect(page.locator(".workloads-panel .mono").filter({ hasText: seed.projectID })).toBeVisible();
  await expect(page.locator(".images-panel .mono").filter({ hasText: seed.projectID })).toBeVisible();
	await expect(page.locator(".usage-panel .mono").filter({ hasText: seed.projectID })).toBeVisible();
	await expect(page.getByRole("table", { name: "ConfigFiles" })).toBeVisible();
	await expect(page.getByRole("table", { name: "Jobs" })).toBeVisible();
	const projectImagesTable = page.getByRole("table", { name: "Project images" });
	await expect(projectImagesTable).toBeVisible();
	if (expectedImageScanStatus) {
		await expect(projectImagesTable.getByRole("cell", { name: expectedImageScanStatus, exact: true })).toBeVisible();
	}
	if (expectedImageState) {
		await expect(projectImagesTable.getByRole("cell", { name: expectedImageState, exact: true })).toBeVisible();
	}
	await expect(page.getByRole("table", { name: "Image builds" })).toBeVisible();
  await expect(page.getByRole("table", { name: "GPU usage" })).toBeVisible();
	await expect(page.getByRole("table", { name: "Request usage" })).toBeVisible();

  const form = page.getByRole("form", { name: "Submit ConfigFile" });
  await form.getByLabel("Name").fill(seed.configName);
  await form.getByLabel("Path").fill(`configs/${seed.configName}`);
  await form.getByLabel("Content").fill(`kind: ConfigMap\nmetadata:\n  name: ${seed.configName}\n`);
  await form.getByRole("button", { name: "Submit ConfigFile" }).click();
  await expect(page.getByText("ConfigFile submitted")).toBeVisible();
  seed.configSubmitted = true;
  await expect(page.getByRole("table", { name: "ConfigFiles" }).getByRole("cell", { name: seed.configName, exact: true })).toBeVisible();

  const jobForm = page.getByRole("form", { name: "Submit Job" });
  await jobForm.getByLabel("Job ID").fill(seed.jobID);
  await jobForm.getByLabel("User ID").fill(seed.userID);
  await jobForm.getByLabel("Queue").fill(seed.queueID);
  await jobForm.getByLabel("CPU").fill("1");
  await jobForm.getByLabel("Memory MB").fill("1024");
  if (seedStreamCredentials) {
    await jobForm.getByLabel("Streaming").check();
    await jobForm.getByLabel("Stream bitrate Kbps").fill("12000");
  }
  const submitResponsePromise = page.waitForResponse(
    (response) => response.url().endsWith("/api/v1/jobs") && response.request().method() === "POST",
  );
  await jobForm.getByRole("button", { name: "Submit Job" }).click();
  const submitResponse = await submitResponsePromise;
  const submitBody = parseJSON(await submitResponse.text());
  const submitData = isRecord(submitBody) ? submitBody.data : undefined;
  if (!submitResponse.ok() || recordID(submitData) !== seed.jobID) {
    throw new Error(`job submit response did not include seeded job id ${seed.jobID}`);
  }
  seed.jobSubmitted = true;
  await expect(page.getByText("Job submitted")).toBeVisible();
  await expect(page.getByRole("table", { name: "Jobs" }).getByRole("cell", { name: seed.jobID, exact: true })).toBeVisible();

  if (seedStreamCredentials) {
    const streamButton = page.getByRole("button", { name: `Open stream ${seed.jobID}` });
    await expect(streamButton).toBeVisible();
    const streamResponsePromise = page.waitForResponse(
      (response) => response.url().endsWith("/api/v1/stream/credentials") && response.request().method() === "POST",
    );
    await streamButton.click();
    const streamResponse = await streamResponsePromise;
    const streamBody = parseJSON(await streamResponse.text());
    const streamData = isEnvelope<unknown>(streamBody) ? streamBody.data : streamBody;
    const turn = streamTurnData(streamData);
    seed.streamCredentialsRequested = true;
    seed.streamCredentialsStatus = streamResponse.status();
    seed.streamCredentialURICount = streamURIList(turn).length;
    seed.streamCredentialUsernamePresent = text(turn?.username).length > 0;
    const streamPassword = text(turn?.password);
    seed.streamCredentialPasswordIssued = streamPassword.length > 0;
    if (!streamResponse.ok()) {
      throw new Error(`stream credential response rejected HTTP ${streamResponse.status()}`);
    }
    const streamDetails = page.getByLabel(`Stream session ${seed.jobID}`);
    await expect(streamDetails.getByText("redacted", { exact: true })).toBeVisible();
    if (streamPassword) {
      await expect(streamDetails.getByText(streamPassword, { exact: true })).toHaveCount(0);
    }
    seed.streamCredentialPasswordRedacted = true;
    if (rtcIceProbe) {
      const rtc = await runRTCICEProbe(page, turn);
      seed.rtcProbeEnvironment = e2eEnvironment;
      seed.rtcDirectOK = rtc.direct.ok;
      seed.rtcDirectCandidateCount = rtc.direct.candidateCount;
      seed.rtcDirectCandidateTypes = rtc.direct.candidateTypes;
      seed.rtcRelayOK = rtc.relay.ok;
      seed.rtcRelayCandidateCount = rtc.relay.candidateCount;
      seed.rtcRelayCandidateTypes = rtc.relay.candidateTypes;
      if (!rtc.direct.ok || !rtc.relay.ok) {
        throw new Error(`RTC ICE probe failed ${JSON.stringify(rtc)}`);
      }
    }
  }

  const logsResponsePromise = page.waitForResponse(
    (response) => response.url().includes(`/api/v1/jobs/${encodeURIComponent(seed.jobID)}/logs`) && response.request().method() === "GET",
  );
  await page.getByRole("button", { name: `View logs ${seed.jobID}` }).click();
  const logsResponse = await logsResponsePromise;
  const logsBody = parseJSON(await logsResponse.text());
  const logsData = isEnvelope<unknown>(logsBody) ? logsBody.data : logsBody;
  seed.jobLogsRequested = true;
  seed.jobLogsStatus = logsResponse.status();
  const logRows = rows(logsData);
  seed.jobLogsCount = logRows.length;
  seed.jobLogsNonempty = logRows.length > 0;
  if (!logsResponse.ok()) {
    throw new Error(`job logs response rejected HTTP ${logsResponse.status()}`);
  }
  const logsTable = page.getByRole("table", { name: `Job logs ${seed.jobID}` });
  await expect(logsTable).toBeVisible();
  const firstLogLine = firstJobLogLine(logRows);
  if (firstLogLine) {
    await expect(logsTable.getByText(firstLogLine, { exact: true }).first()).toBeVisible();
    seed.jobLogsVisible = true;
  } else {
    seed.jobLogsVisible = false;
  }
  if (expectNonemptyLogs && !seed.jobLogsVisible) {
    throw new Error(`expected visible non-empty job logs for ${seed.jobID}`);
  }

  const cancelResponsePromise = page.waitForResponse(
    (response) => response.url().includes(`/api/v1/jobs/${encodeURIComponent(seed.jobID)}/cancel`) && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: `Cancel ${seed.jobID}` }).click();
  const cancelResponse = await cancelResponsePromise;
  const cancelBody = parseJSON(await cancelResponse.text());
  const cancelData = isRecord(cancelBody) ? cancelBody.data : undefined;
  if (!cancelResponse.ok()) {
    throw new Error(`job cancel response rejected HTTP ${cancelResponse.status()}`);
  }
  seed.jobCancelRequested = true;
  seed.jobCancelCommandID = recordID(cancelData);
	await expect(page.getByText("Cancel requested")).toBeVisible();
}

async function assertExpectedProjectImageStatusUI(page: Page) {
	const projectSelect = page.getByLabel("Active project");
	await expect(projectSelect).toBeVisible();
	if (expectedProjectName) {
		await expect(projectSelect).toContainText(expectedProjectName);
	}
	await projectSelect.selectOption(expectedProjectID);
	await expect(projectSelect).toHaveValue(expectedProjectID);

	const projectImagesTable = page.getByRole("table", { name: "Project images" });
	await expect(projectImagesTable).toBeVisible();
	if (expectedImageScanStatus) {
		await expect(projectImagesTable.getByRole("cell", { name: expectedImageScanStatus, exact: true })).toBeVisible();
	}
	if (expectedImageState) {
		await expect(projectImagesTable.getByRole("cell", { name: expectedImageState, exact: true })).toBeVisible();
	}
}

async function captureRouteProof(request: APIRequestContext, seed: SeedState): Promise<void> {
  const [projects, configFiles, jobs, images, builds] = await Promise.all([
    apiRequest<unknown[]>(request, "GET", "/api/v1/projects"),
    apiRequest<unknown[]>(request, "GET", `/api/v1/projects/${encodeURIComponent(seed.projectID)}/config-files`),
    apiRequest<unknown[]>(request, "GET", "/api/v1/jobs"),
    apiRequest<unknown[]>(request, "GET", `/api/v1/projects/${encodeURIComponent(seed.projectID)}/images`),
    apiRequest<unknown[]>(request, "GET", `/api/v1/projects/${encodeURIComponent(seed.projectID)}/image-builds`),
  ]);
  const gpuUsage = expectNonzeroGPU
    ? await waitForProjectGPUUsage(request, seed.projectID)
    : await projectGPUUsage(request, seed.projectID);
  const configRows = rows(configFiles.data);
  const jobRows = rows(jobs.data);
  const imageRows = rows(images.data);
  const gpuUsed = projectGPUUsed(gpuUsage.data);
  const seededConfig = configRows.find((row) => recordName(row) === seed.configName);
  const seededImage = imageRows.find((row) => imageIdentifier(row) !== "");
  seed.configID = recordID(seededConfig);
  seed.imageRuleID = imageIdentifier(seededImage);
  seed.gpuUsed = gpuUsed;
  seed.gpuNonzero = gpuUsed > 0;
		const proof = {
				project_id: seed.projectID,
				project_count: rows(projects.data).length,
    seeded_project_present: rows(projects.data).some((row) => recordID(row) === seed.projectID),
    config_file_count: configRows.length,
    seeded_config_id: seed.configID || "missing",
    job_count: jobRows.length,
    seeded_job_present: jobRows.some((row) => recordID(row) === seed.jobID),
    seeded_job_streaming: jobRows.some((row) => recordID(row) === seed.jobID && boolValue(rowData(row).streaming_session)),
    job_cancel_requested: seed.jobCancelRequested === true,
    job_cancel_command_id: seed.jobCancelCommandID || "missing",
    job_logs_requested: seed.jobLogsRequested === true,
    job_logs_status: seed.jobLogsStatus ?? "missing",
    job_logs_count: seed.jobLogsCount ?? "missing",
    job_logs_nonempty: seed.jobLogsNonempty === true,
    job_logs_visible: seed.jobLogsVisible === true,
    stream_credentials_requested: seed.streamCredentialsRequested === true,
    stream_credentials_status: seed.streamCredentialsStatus ?? "missing",
    stream_credential_uri_count: seed.streamCredentialURICount ?? "missing",
    stream_credential_username_present: seed.streamCredentialUsernamePresent === true,
    stream_credential_password_issued: seed.streamCredentialPasswordIssued === true,
    stream_credential_password_redacted: seed.streamCredentialPasswordRedacted === true,
    rtc_probe_requested: rtcIceProbe,
    rtc_probe_environment: rtcIceProbe ? seed.rtcProbeEnvironment || "missing" : "not-requested",
    rtc_direct_ok: seed.rtcDirectOK === true,
    rtc_direct_candidate_count: seed.rtcDirectCandidateCount ?? 0,
    rtc_direct_candidate_types: seed.rtcDirectCandidateTypes ?? [],
    rtc_relay_ok: seed.rtcRelayOK === true,
    rtc_relay_candidate_count: seed.rtcRelayCandidateCount ?? 0,
    rtc_relay_candidate_types: seed.rtcRelayCandidateTypes ?? [],
			image_count: imageRows.length,
			seeded_image_identifier: seed.imageRuleID || "missing",
			seeded_image_scan_status: imageScanStatus(seededImage) || "missing",
			seeded_image_state: imageDisplayState(seededImage) || "missing",
			build_count: rows(builds.data).length,
			gpu_status: gpuUsage.status,
			gpu_ok: gpuUsage.ok,
    gpu_used: seed.gpuUsed,
    gpu_nonzero: seed.gpuNonzero === true,
		};
  console.log(`nexuspaas live seeded route proof ${JSON.stringify(proof)}`);
  if (expectNonzeroGPU && !seed.gpuNonzero) {
    throw new Error(`expected nonzero Project GPU usage for ${seed.projectID}, got ${gpuUsed}`);
  }
}

async function waitForProjectGPUUsage(request: APIRequestContext, projectID: string): Promise<APIResult<unknown>> {
  let result = await projectGPUUsage(request, projectID);
  for (let attempt = 1; attempt < 10 && result.ok && projectGPUUsed(result.data) <= 0; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 1000));
    result = await projectGPUUsage(request, projectID);
  }
  return result;
}

async function projectGPUUsage(request: APIRequestContext, projectID: string): Promise<APIResult<unknown>> {
  return apiRequest<unknown>(request, "GET", `/api/v1/projects/${encodeURIComponent(projectID)}/gpu-usage`);
}

async function cleanupSeed(request: APIRequestContext, seed: SeedState): Promise<void> {
  const cleanup: string[] = [];
  if (!seed.configID && seed.configSubmitted) {
    seed.configID = await findSeedConfigID(request, seed);
  }
  if (seed.configID) {
    cleanup.push(await cleanupDelete(request, `/api/v1/configfiles/${encodeURIComponent(seed.configID)}`, `configfile=${seed.configID}`));
  } else if (seed.configSubmitted) {
    seed.leftovers.push(`configfile name=${seed.configName} cleanup skipped because route proof did not expose an id`);
  }
  if (seed.imageBuildCreated && seed.imageBuildID) {
    cleanup.push(
      await cleanupDelete(
        request,
        `/api/v1/projects/${encodeURIComponent(seed.projectID)}/image-builds/${encodeURIComponent(seed.imageBuildID)}`,
        `image_build=${seed.imageBuildID}`,
      ),
    );
  }
  const imageRuleCleanupID = seed.imageRuleID || (seed.imageRequestCreated ? seed.imageReference : "");
  if (imageRuleCleanupID) {
    cleanup.push(
      await cleanupDelete(
        request,
        `/api/v1/projects/${encodeURIComponent(seed.projectID)}/images/${encodeURIComponent(imageRuleCleanupID)}`,
        `project_image=${imageRuleCleanupID}`,
      ),
    );
  }
  if (seed.imageRequestCreated && seed.imageRequestID) {
    seed.leftovers.push(`image_request=${seed.imageRequestID} has no DELETE route`);
  }
  if (seed.jobSubmitted) {
    seed.leftovers.push(`job=${seed.jobID} has no DELETE route`);
  }
  if (seed.jobCancelRequested) {
    seed.leftovers.push(`job_cancel_command=${seed.jobCancelCommandID || seed.jobID} has no DELETE route`);
  }
  if (seed.planCreated) {
    cleanup.push(await cleanupDelete(request, `/api/v1/plans/${encodeURIComponent(seed.planID)}`, `plan=${seed.planID}`));
  } else if (seed.planBound) {
    seed.leftovers.push(`plan_binding=${seed.planID} cleanup skipped because plan was not created`);
  }
  if (seed.queueCreated) {
    cleanup.push(await cleanupDelete(request, `/api/v1/queues/${encodeURIComponent(seed.queueID)}`, `queue=${seed.queueID}`));
  }
  if (seed.projectCreated) {
    cleanup.push(await cleanupDelete(request, `/api/v1/projects/${encodeURIComponent(seed.projectID)}`, `project=${seed.projectID}`));
  }
  if (seed.groupCreated) {
    cleanup.push(await cleanupDelete(request, `/api/v1/groups/${encodeURIComponent(seed.groupID)}`, `group=${seed.groupID}`));
  }
  console.log(`nexuspaas live seed cleanup ${JSON.stringify({ project_id: seed.projectID, cleanup, leftovers: seed.leftovers })}`);
}

async function findSeedConfigID(request: APIRequestContext, seed: SeedState): Promise<string> {
  const result = await apiRequest<unknown[]>(request, "GET", `/api/v1/projects/${encodeURIComponent(seed.projectID)}/config-files`);
  if (!result.ok) {
    return "";
  }
  const config = rows(result.data).find((row) => recordName(row) === seed.configName);
  return recordID(config);
}

async function cleanupDelete(request: APIRequestContext, path: string, label: string): Promise<string> {
  const result = await apiRequest<unknown>(request, "DELETE", path);
  if (result.ok || result.status === 404) {
    return `${label}: HTTP ${result.status}`;
  }
  return `${label}: cleanup rejected HTTP ${result.status} ${result.error ?? ""}`.trim();
}

async function apiJSON<T = unknown>(request: APIRequestContext, method: APIMethod, path: string, data?: unknown): Promise<T> {
  const result = await apiRequest<T>(request, method, path, data);
  if (!result.ok) {
    throw new Error(`${method} ${path} failed with HTTP ${result.status}: ${result.error ?? "request failed"}`);
  }
  return result.data as T;
}

async function apiRequest<T>(request: APIRequestContext, method: APIMethod, path: string, data?: unknown): Promise<APIResult<T>> {
  const response = await request.fetch(apiURL(path), {
    method,
    headers: {
      Accept: "application/json",
      "X-API-Key": apiKey!,
      ...(data === undefined ? {} : { "Content-Type": "application/json" }),
    },
    ...(data === undefined ? {} : { data }),
  });
  return readAPIResponse<T>(response);
}

async function readAPIResponse<T>(response: APIResponse): Promise<APIResult<T>> {
  const text = await response.text();
  const body = parseJSON(text);
  const envelope = isEnvelope<T>(body) ? body : undefined;
  const data = envelope ? envelope.data : (body as T | undefined);
  const ok = response.ok() && envelope?.success !== false;
  return {
    ok,
    status: response.status(),
    data,
    error: envelope?.error?.message || envelopeDataMessage(envelope?.data) || response.statusText(),
  };
}

function apiURL(path: string): string {
  const root = apiBaseURL || appOrigin(appPath);
  if (!root) {
    return path;
  }
  return new URL(path, root).toString();
}

function appOrigin(value: string): string {
  try {
    return new URL(value).origin;
  } catch {
    return "";
  }
}

function parseJSON(text: string): unknown {
  if (!text.trim()) {
    return undefined;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return text;
  }
}

function isEnvelope<T>(value: unknown): value is APIEnvelope<T> {
  return isRecord(value) && ("success" in value || "data" in value || "error" in value);
}

function envelopeDataMessage(value: unknown): string {
  return isRecord(value) ? text(value.message) : "";
}

function rows(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value;
  }
  if (isRecord(value) && Array.isArray(value.items)) {
    return value.items;
  }
  return [];
}

function recordID(value: unknown): string {
  if (!isRecord(value)) {
    return "";
  }
  const data = isRecord(value.data) ? value.data : value;
  return text(value.id) || text(data.id) || text(data.project_id) || text(data.p_id) || text(data.PID);
}

function recordName(value: unknown): string {
  if (!isRecord(value)) {
    return "";
  }
  const data = isRecord(value.data) ? value.data : value;
  return text(data.name) || text(data.project_name) || text(data.ProjectName) || text(data.path);
}

function imageIdentifier(value: unknown): string {
	if (!isRecord(value)) {
		return "";
	}
	return text(value.id) || text(value.tag_id) || text(value.tagId) || text(value.image_reference) || text(value.imageReference);
}

function imageScanStatus(value: unknown): string {
	const data = rowData(value);
	return text(data.scan_status) || text(data.scanStatus);
}

function imageDisplayState(value: unknown): string {
	const data = rowData(value);
	if (boolValue(data.deleted)) {
		return "deleted";
	}
	if (boolValue(data.unavailable)) {
		return "unavailable";
	}
	return text(data.status);
}

function streamTurnData(value: unknown): Record<string, unknown> | undefined {
	const data = rowData(value);
	return isRecord(data.turn) ? data.turn : undefined;
}

function streamURIList(value: Record<string, unknown> | undefined): string[] {
	if (!value || !Array.isArray(value.uris)) {
		return [];
	}
	return value.uris.filter((uri): uri is string => typeof uri === "string" && uri.length > 0);
}

async function runRTCICEProbe(page: Page, turn: Record<string, unknown> | undefined): Promise<RTCProbeSummary> {
	const turnURIs = streamURIList(turn).filter((uri) => uri.startsWith("turn:") || uri.startsWith("turns:"));
	const username = text(turn?.username);
	const credential = text(turn?.password);
	return page.evaluate(
		async ({ turnURIs, username, credential }) => {
			type BrowserRTCProbeResult = {
				ok: boolean;
				completed: boolean;
				candidateCount: number;
				candidateTypes: string[];
				error?: string;
			};

			async function gather(config: RTCConfiguration, expectedType?: string): Promise<BrowserRTCProbeResult> {
				const candidates: string[] = [];
				const types = new Set<string>();
				let pc: RTCPeerConnection | undefined;
				return new Promise<BrowserRTCProbeResult>((resolve) => {
					let settled = false;
					const finish = (completed: boolean, error?: string) => {
						if (settled) {
							return;
						}
						settled = true;
						window.clearTimeout(timer);
						pc?.close();
						resolve({
							ok: candidates.length > 0 && (!expectedType || types.has(expectedType)),
							completed,
							candidateCount: candidates.length,
							candidateTypes: Array.from(types).sort(),
							...(error ? { error } : {}),
						});
					};
					const timer = window.setTimeout(() => finish(false), 5000);
					try {
						pc = new RTCPeerConnection(config);
						pc.createDataChannel("nexuspaas-ice-probe");
						pc.onicecandidate = (event) => {
							const candidate = event.candidate?.candidate;
							if (!candidate) {
								return;
							}
							candidates.push(candidate);
							const match = /\btyp\s+([a-z0-9]+)/i.exec(candidate);
							if (match?.[1]) {
								types.add(match[1].toLowerCase());
							}
						};
						pc.onicegatheringstatechange = () => {
							if (pc?.iceGatheringState === "complete") {
								finish(true);
							}
						};
						pc.createOffer()
							.then((offer) => pc?.setLocalDescription(offer))
							.catch((error: unknown) => finish(false, error instanceof Error ? error.message : String(error)));
					} catch (error) {
						finish(false, error instanceof Error ? error.message : String(error));
					}
				});
			}

			const direct = await gather({});
			const relay =
				turnURIs.length > 0 && username && credential
					? await gather(
							{
								iceServers: [{ urls: turnURIs, username, credential }],
								iceTransportPolicy: "relay",
							},
							"relay",
						)
					: { ok: false, completed: false, candidateCount: 0, candidateTypes: [], error: "TURN credentials missing" };
			return { direct, relay };
		},
		{ turnURIs, username, credential },
	);
}

function imageBuildMatchesSeed(value: unknown, seed: SeedState): boolean {
	if (!isRecord(value)) {
		return false;
  }
  const data = isRecord(value.data) ? value.data : value;
  const id = text(data.id) || text(data.build_id) || text(data.buildId) || text(data.job_name) || text(data.jobName);
  const projectID = text(data.project_id) || text(data.projectId) || text(data.p_id) || text(data.PID);
  return id === seed.imageBuildID && projectID === seed.projectID;
}

function firstJobLogLine(values: unknown[]): string {
	for (const value of values) {
		const data = rowData(value);
		const line = text(data.line) || text(data.message) || text(data.text) || text(data.content) || recordID(value);
		if (line) {
			return line;
		}
	}
	return "";
}

function projectGPUUsed(value: unknown): number {
	const data = rowData(value);
	return numberValue(data.used) || numberValue(data.Used);
}

function numberValue(value: unknown): number {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}
	if (typeof value === "string") {
		const parsed = Number(value);
		return Number.isFinite(parsed) ? parsed : 0;
	}
	return 0;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

function rowData(value: unknown): Record<string, unknown> {
	if (!isRecord(value)) {
		return {};
	}
	return isRecord(value.data) ? value.data : value;
}

function text(value: unknown): string {
	return typeof value === "string" ? value : "";
}

function boolValue(value: unknown): boolean {
	return value === true || value === "true";
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

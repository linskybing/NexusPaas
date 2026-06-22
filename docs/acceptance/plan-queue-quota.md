# Core Feature Area C: Plan, Queue, Quota, and Preemption

Part of the [GA Acceptance docs](README.md).

## Goal

Every ConfigFile deployment and image build must pass through queue admission.

A Project can deploy only if its active Plan allows the requested Queue, time
window, CPU, RAM, GPU type, GPU amount, streaming session, and build resource
usage.

## Plan Model

```text
Plan
- id
- name
- allowed_queue_ids
- allowed_gpu_types
- allowed_device_classes
- cpu_core_limit
- memory_gib_limit
- gpu_count_limit
- gpu_sm_total_limit
- gpu_memory_gib_limit
- max_running_jobs
- max_running_builds
- max_stream_sessions
- allowed_time_windows
- starts_at
- expires_at
- build_cpu_limit
- build_memory_gib_limit
- build_time_limit_seconds
- created_by
- updated_by
```

## Queue Model

```text
Queue
- id
- name
- priority_value
- is_preemptible
- can_preempt
- max_runtime_seconds
- allowed_gpu_types
- allowed_device_classes
- max_cpu_per_job
- max_memory_gib_per_job
- max_gpu_per_job
- max_gpu_sm_percentage_per_job
- stream_session_cap
- stream_egress_budget_kbps
- active_windows
```

## Admission Flow

```text
Submit request
  -> check user Project access
  -> check active Project Plan
  -> check Plan is not expired
  -> check current time inside Plan time window
  -> check Queue is allowed by Plan
  -> check requested CPU/RAM/GPU against Queue limits
  -> check requested CPU/RAM/GPU against Project Plan remaining quota
  -> check image allow list
  -> check storage mount plan
  -> check WebRTC streaming limits if requested
  -> reserve quota
  -> submit to Kueue / Kubernetes path
  -> commit reservation when accepted
  -> release quota on terminal state
```

## Preemption Flow

```text
High-priority workload cannot fit
  -> scheduler-quota checks requester queue can_preempt=true
  -> find lower-priority workloads in preemptible queues
  -> exclude non-preemptible workloads
  -> exclude equal/higher priority workloads
  -> choose minimum victim set
  -> call workload-service preempt command
  -> k8s-control deletes victim resources
  -> victim quota released
  -> requester admission retried
  -> audit events and notifications emitted
```

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| QUEUE-001 | A Project without an active Plan cannot deploy workloads. |
| QUEUE-002 | A Project with an expired Plan cannot deploy new workloads. |
| QUEUE-003 | A Project outside its allowed Plan time window cannot deploy new workloads. |
| QUEUE-004 | A user cannot select a Queue not included in the Project Plan. |
| QUEUE-005 | A workload exceeding Queue CPU/RAM/GPU limits is rejected. |
| QUEUE-006 | A workload exceeding remaining Project quota is rejected. |
| QUEUE-007 | All successful submissions create a quota reservation before Kubernetes apply. |
| QUEUE-008 | Quota is committed when workload is accepted by compute-control-plane. |
| QUEUE-009 | Quota is released when workload reaches terminal state. |
| QUEUE-010 | Runtime limit is enforced by Kubernetes and by NexusPaaS runtime reaper. |
| QUEUE-011 | High-priority Queue can preempt lower-priority preemptible workload when allowed. |
| QUEUE-012 | High-priority Queue cannot preempt non-preemptible workload. |
| QUEUE-013 | High-priority Queue cannot preempt equal or higher priority workload. |
| QUEUE-014 | Preemption selects the minimum sufficient victim set. |
| QUEUE-015 | Preempted workloads are marked preempted and produce audit events. |
| QUEUE-016 | Preempted resources are deleted from Kubernetes. |
| QUEUE-017 | Preempted quota is released exactly once. |
| QUEUE-018 | Scheduler restart does not lose reservations. |
| QUEUE-019 | Reconciler can repair reservation drift after service restart. |
| QUEUE-020 | Queue metrics expose pending, admitted, running, preempted, rejected, and admission latency. |
| QUEUE-021 | E2E test covers plan window, runtime expiry, quota rejection, and preemption. |
| QUEUE-022 | Kueue or equivalent Kubernetes-native queueing integration is the default GA path; custom queue logic may only wrap policy and auditing. |

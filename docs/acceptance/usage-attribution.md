# Core Feature Area E: Container, PID, Process, and GPU Usage Attribution

Part of the [GA Acceptance docs](README.md).

## Goal

NexusPaaS must attribute usage to the correct user, Project, and Group.

The primary attribution unit is the container.

Process-level metrics are used to improve granularity, especially for GPU MPS
sharing and multi-process workloads.

## Key Decision

For MPS sharing on one physical GPU, the platform must distinguish users by
mapping GPU process PIDs back to containers, then mapping containers back to
Pods, Jobs, Projects, Groups, and Users.

The simplest reliable ownership chain is:

```text
GPU process PID
  -> host /proc metadata
  -> cgroup path
  -> container ID
  -> CRI container metadata
  -> Kubernetes Pod UID
  -> NexusPaaS Job ID
  -> Project ID
  -> Group ID
  -> User ID
```

PID alone is not sufficient because PIDs can be reused.

The required process identity key is:

```text
node_name
+ pid
+ process_start_time
+ pid_namespace_inode
+ cgroup_path
+ container_id
```

## Required Node-Level Components

Each GPU node must run a NexusPaaS usage attribution stack.

```text
nexus-usage-agent DaemonSet
  - reads Kubernetes Pod metadata
  - reads CRI/container runtime metadata
  - reads /proc and cgroup metadata
  - maps PID -> container -> pod -> job -> user/project/group
  - samples NVIDIA GPU process usage through NVML or nvidia-smi where available
  - enriches process-exporter metrics with NexusPaaS ownership
  - exports Prometheus metrics
  - writes usage samples or rollups to usage-observability

process-exporter DaemonSet or sidecar
  - mines /proc for process CPU/RAM/file/thread metrics
  - groups watched processes
  - provides process-level metrics to Prometheus

dcgm-exporter DaemonSet
  - exports GPU-level metrics and health
  - provides GPU UUID, GPU utilization, memory, power, temperature, encoder/decoder metrics
```

## Why process-exporter Is Needed

Container-level CPU/RAM usage is useful but not enough for detailed attribution.

process-exporter is required for:

- Per-process CPU seconds.
- Per-process RSS / memory where available.
- Process count and lifecycle visibility.
- Debugging multi-process containers.
- Separating heavy child processes inside the same container.
- Supporting process-level attribution views for administrators.

However, process-exporter should not be the only source of ownership truth.

Ownership truth must come from Kubernetes labels, Pod UID, container ID, cgroup,
and Job metadata.

## GPU Process Sampling

The platform must collect GPU process information on GPU nodes. Required fields:

```text
timestamp
node_name
gpu_uuid
gpu_index
pid
process_start_time
process_name
container_id
pod_uid
namespace
pod_name
container_name
job_id
user_id
project_id
group_id
queue_id
plan_id
gpu_memory_used_bytes
gpu_sm_utilization_ratio
gpu_sm_utilization_source
gpu_memory_source
mps_server_pid
mps_client_pid
mps_reserved_sm_percentage
mps_pinned_memory_limit
```

## Metric Source Rules

| Metric | Preferred Source | Fallback Source | Billing Source |
|---|---|---|---|
| CPU usage | process-exporter + cgroup/container metrics | kubelet/cAdvisor | reservation or actual by policy |
| Memory usage | process-exporter RSS + cgroup/container metrics | kubelet/cAdvisor | reservation or actual by policy |
| GPU memory used | NVML / nvidia-smi process query | DCGM where process-level available | observed or reserved by policy |
| GPU SM observed | NVML/DCGM per-process if available | estimated by MPS reservation share | reserved SM-hours |
| GPU SM reserved | scheduler admission | DRA/MPS config | reserved SM-hours |
| GPU health | DCGM exporter | NVIDIA tooling | not billable |
| Encoder/decoder usage | DCGM exporter where available | unavailable | observed only |

## MPS Attribution Rules

For a shared MPS GPU:

- Every admitted workload has a reserved SM percentage.
- Every running container has one or more host PIDs.
- GPU process PIDs are mapped to containers.
- GPU memory usage is attributed to the owning container when process-level memory is available.
- Actual per-process SM usage must be recorded when supported by the NVIDIA stack.
- If actual per-process SM usage is not available, the platform must show:
  - reserved SM ratio
  - estimated SM ratio
  - metric source = `estimated_mps_allocation`
- Billing and quota must not depend on unsupported or ambiguous MPS per-process SM metrics.
- The MPS server process overhead must not be charged entirely to one user.
- MPS server overhead may be recorded as system overhead or distributed proportionally by reserved SM share.
- UI must clearly distinguish:
  - reserved
  - observed
  - estimated
  - unavailable

## Container Ownership Labels

All workload Pods must contain:

```yaml
metadata:
  labels:
    nexuspaas.io/user-id: "<user_id>"
    nexuspaas.io/project-id: "<project_id>"
    nexuspaas.io/group-id: "<group_id>"
    nexuspaas.io/job-id: "<job_id>"
    nexuspaas.io/queue-id: "<queue_id>"
    nexuspaas.io/plan-id: "<plan_id>"
    nexuspaas.io/config-version: "<config_version>"
    nexuspaas.io/workload-kind: "job|deployment|pod|ide|stream"
```

All containers must be traceable through:

- Pod UID
- container ID
- container name
- image digest
- job ID
- user ID
- project ID
- group ID

## Usage Metrics

The platform should export metrics similar to:

```text
nexuspaas_container_cpu_usage_seconds_total
nexuspaas_container_memory_working_set_bytes
nexuspaas_process_cpu_seconds_total
nexuspaas_process_resident_memory_bytes
nexuspaas_process_count
nexuspaas_gpu_process_memory_used_bytes
nexuspaas_gpu_process_sm_utilization_ratio
nexuspaas_gpu_mps_reserved_sm_ratio
nexuspaas_gpu_mps_effective_gpu
nexuspaas_gpu_attribution_sample_total
nexuspaas_gpu_attribution_orphan_pid_total
nexuspaas_gpu_attribution_unknown_container_total
nexuspaas_gpu_attribution_stale_sample_total
```

Required labels:

```text
node
gpu_uuid
pid
container_id
pod_uid
namespace
pod
container
job_id
user_id
project_id
group_id
queue_id
plan_id
source
```

High-cardinality labels such as `pid` must be controlled through retention,
recording rules, and rollup tables.

## Usage Data Products

### Real-Time Usage

Real-time view must show:

| Dimension | Required Data |
|---|---|
| User | active jobs, CPU, RAM, reserved GPU, observed GPU memory, observed/estimated SM |
| Project | active jobs, active streams, quota used, GPU used |
| Group | current aggregate CPU/RAM/GPU usage |
| Job | container list, process list, GPU process list |
| Node | GPU health, MPS clients, orphan processes |
| Queue | running, pending, preempted, rejected |

### Historical Usage

Historical rollups must support:

```text
user_id
project_id
group_id
job_id
queue_id
plan_id
gpu_uuid
time range
step
```

Required totals:

```text
cpu_core_hours
memory_gib_hours
gpu_reserved_sm_hours
gpu_observed_memory_gib_hours
gpu_estimated_sm_hours
gpu_whole_card_hours
stream_session_hours
build_cpu_core_hours
build_memory_gib_hours
```

## Acceptance Criteria

| ID | Acceptance Criteria |
|---|---|
| USAGE-001 | Every platform-created workload Pod has NexusPaaS ownership labels. |
| USAGE-002 | Every running container can be mapped to a Pod UID, Job ID, User ID, Project ID, and Group ID within 30 seconds. |
| USAGE-003 | nexus-usage-agent runs on every GPU node. |
| USAGE-004 | process-exporter runs on every node where process-level accounting is required. |
| USAGE-005 | DCGM exporter runs on every GPU node. |
| USAGE-006 | GPU process PID is mapped to container ID through cgroup or CRI metadata. |
| USAGE-007 | PID reuse is handled using process start time and namespace/cgroup identity. |
| USAGE-008 | Two users sharing one GPU through MPS are shown as separate usage owners. |
| USAGE-009 | Two containers inside the same Pod can be distinguished. |
| USAGE-010 | Multiple processes inside the same container are aggregated correctly to that container. |
| USAGE-011 | Child processes are attributed to the same container owner. |
| USAGE-012 | GPU process memory usage is attributed to the correct container when available. |
| USAGE-013 | MPS reserved SM ratio is attributed to the correct container and user. |
| USAGE-014 | Observed per-process SM ratio is recorded when supported by the node NVIDIA stack. |
| USAGE-015 | If observed per-process SM ratio is unavailable, metric source is marked `estimated_mps_allocation` or unavailable. |
| USAGE-016 | UI never labels estimated MPS SM usage as measured actual usage. |
| USAGE-017 | Billing-grade GPU SM-hours use reserved/admitted MPS allocation unless true measured per-process SM is validated. |
| USAGE-018 | MPS server process overhead is not charged entirely to one user. |
| USAGE-019 | Orphan GPU PIDs are reported as system/orphan and alerted if not resolved within the threshold. |
| USAGE-020 | Terminated containers stop accumulating active usage after termination is observed. |
| USAGE-021 | Container restart with a new container ID does not double count previous usage. |
| USAGE-022 | Short-lived processes may be best-effort observed, but billing is not dependent on catching every short-lived PID sample. |
| USAGE-023 | Prometheus high-cardinality process metrics are rolled up before long-term storage. |
| USAGE-024 | Group usage reports can aggregate all Project and User usage over an arbitrary time range. |
| USAGE-025 | Project admin can view Project usage but cannot view unrelated Projects. |
| USAGE-026 | Group admin can view Group usage but cannot view unrelated Groups. |
| USAGE-027 | Platform admin can view cluster-wide usage. |
| USAGE-028 | Usage rollup can be rebuilt from raw samples or workload lifecycle records. |
| USAGE-029 | Usage drift between scheduler reservation and observed telemetry creates an alert. |
| USAGE-030 | Usage API supports from, to, step, group_id, project_id, user_id, and job_id filters. |
| USAGE-031 | Usage API returns source fields so consumers know whether metrics are reserved, observed, estimated, or unavailable. |
| USAGE-032 | Node agent failure marks telemetry stale but does not block quota release. |
| USAGE-033 | Missing telemetry never grants extra quota. |
| USAGE-034 | Process telemetry access requires platform admin or scoped Project/Group permission. |
| USAGE-035 | E2E test starts two MPS workloads on one GPU and verifies separate user/project attribution. |
| USAGE-036 | E2E test verifies process-exporter metrics can be joined to container ownership. |
| USAGE-037 | E2E test verifies GPU PID mapping to container ID and user ID. |

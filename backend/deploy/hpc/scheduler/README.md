# HPC Scheduler Overlay

Reference manifests for scheduler backends used by seeded PlacementProfiles.

This directory does not install Kueue or Volcano. Install the controllers and
CRDs first, then adapt these queue templates to your cluster capacity.

Seeded profile alignment:

- `kueue-batch` injects `kueue.x-k8s.io/queue-name=<queue>` onto workload
  metadata. The queue name must match a `LocalQueue` in the workload namespace.
- `volcano-gang` resolves `scheduler_name=volcano`; workload dispatch already
  synthesizes Volcano `Job`/`PodGroup` resources when the Volcano API is present.

Verify syntax locally:

```bash
ruby -e 'require "yaml"; Dir["backend/deploy/hpc/scheduler/*.yaml"].each { |f| YAML.load_stream(File.read(f)); puts f }'
kubectl apply --dry-run=client -f backend/deploy/hpc/scheduler/
```

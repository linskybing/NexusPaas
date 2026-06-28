# NexusPaaS HPC Storage Overlay

This directory declares the StorageClasses referenced by the seeded storage-service profiles.

| StorageClass | Seeded profile | Role |
| --- | --- | --- |
| `local-nvme-scratch` | `local-nvme-scratch` | Node-local hot scratch/cache; requires local PVs provisioned outside this overlay. |
| `cephfs-rwx-authority` | `cephfs-rwx-authority` | RWX authority tier for shared datasets and checkpoint flush targets. |
| `longhorn-rwx-standard` | `longhorn-rwx-standard` | Shared compatibility tier; not the training hot path. |

`deploy/k3s` remains the local/dev baseline. Apply these manifests only to clusters that have the matching CSI drivers and backing storage installed.

Validation:

```sh
kubectl apply --dry-run=client -f backend/deploy/hpc/storage/
```

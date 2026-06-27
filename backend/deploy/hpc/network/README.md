# HPC Network Overlay

This directory contains the reference network objects matched by the seeded
`rdma-fast-lane` NetworkProfile.

Prerequisites are intentionally not bundled here:

- Multus CNI
- SR-IOV CNI
- RDMA CNI
- Whereabouts IPAM, or an equivalent IPAM plugin
- SR-IOV/RDMA device plugin publishing `rdma/rdma_shared_device_a`

`multus-rdma-net.yaml` creates `nexuspaas-system/rdma-net`, the secondary
network injected when scheduler admission resolves `network_profile:
rdma-fast-lane`.

Verify syntax locally:

```bash
ruby -e 'require "yaml"; Dir["backend/deploy/hpc/network/*.yaml"].each { |f| YAML.load_stream(File.read(f)); puts f }'
kubectl apply --dry-run=client -f backend/deploy/hpc/network/
```

If your cluster uses a different interface, resource name, or IP range, update
the NetworkProfile record and this manifest together.

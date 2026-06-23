# Selkies Stream Sidecar Image

Generic GPU streaming **sidecar** for NexusPaas. It runs the X display server +
NVENC capture + WebRTC signaling and is auto-injected next to the user's app
container (the app renders into the shared `DISPLAY :0`). Users never bake
Selkies into their own image.

Build and publish, then point `STREAM_SIDECAR_IMAGE` at the tag:

```bash
docker build -t registry.example.com/nexuspaas/selkies-gl-desktop:24.04 backend/streaming/selkies-gl-desktop
docker push registry.example.com/nexuspaas/selkies-gl-desktop:24.04
```

Defaults:

- 1920x1080 @ 60 FPS, H.264 via NVIDIA NVENC (`SELKIES_ENCODER=nvh264enc`).
- 12 Mbps cap (`STREAM_MAX_BITRATE_KBPS` / `SELKIES_VIDEO_BITRATE`); dispatch
  overrides these from the job's `stream_max_bitrate_kbps`.
- Signaling on `:8080`, metrics on `:9090`.
- Basic auth disabled — sessions are authorized through gateway/Ingress.

This is the generic GL desktop baseline. App-specific runtimes (Isaac Sim,
Omniverse Kit) stay in the **user's** app image; the sidecar is unchanged.

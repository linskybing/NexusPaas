# Selkies GL Desktop Image

Generic browser GPU desktop image for NexusPaas streaming jobs.

Build and publish:

```bash
docker build -t registry.example.com/nexuspaas/selkies-gl-desktop:24.04 backend/streaming/selkies-gl-desktop
docker push registry.example.com/nexuspaas/selkies-gl-desktop:24.04
```

Defaults:

- 1920x1080 at 60 FPS.
- H.264 through NVIDIA NVENC: `SELKIES_ENCODER=nvh264enc`.
- 12 Mbps cap: `STREAM_MAX_BITRATE_KBPS=12000` and `SELKIES_VIDEO_BITRATE=12000`.
- Web UI/signaling on `:8080`; Selkies metrics port set to `:9090`.
- Selkies basic auth disabled because NexusPaas routes the session through gateway/Ingress auth.

App-specific layers such as Isaac Sim or Omniverse Kit should be separate image
tags when needed. Keep this image as the generic GL desktop baseline.

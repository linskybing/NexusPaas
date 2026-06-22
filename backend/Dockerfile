FROM node:24.15.0-alpine AS web-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/index.html frontend/tsconfig.json frontend/vite.config.ts ./
COPY frontend/src ./src
RUN npm run build

FROM golang:1.25.11-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/cmd ./cmd
COPY backend/internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/microservice ./cmd/microservice

FROM alpine:3.22
RUN apk add --no-cache --upgrade ca-certificates libcrypto3 libssl3 \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=build /out/microservice /usr/local/bin/microservice
WORKDIR /app
COPY --from=web-build --chown=app:app /src/frontend/dist ./web
COPY --chown=app:app backend/audit-compliance-service/migrations ./audit-compliance-service/migrations
COPY --chown=app:app backend/authorization-policy-service/migrations ./authorization-policy-service/migrations
COPY --chown=app:app backend/ide-service/migrations ./ide-service/migrations
COPY --chown=app:app backend/identity-service/migrations ./identity-service/migrations
COPY --chown=app:app backend/image-registry-service/migrations ./image-registry-service/migrations
COPY --chown=app:app backend/integration-proxy-service/migrations ./integration-proxy-service/migrations
COPY --chown=app:app backend/k8s-control-service/migrations ./k8s-control-service/migrations
COPY --chown=app:app backend/media-upload-service/migrations ./media-upload-service/migrations
COPY --chown=app:app backend/org-project-service/migrations ./org-project-service/migrations
COPY --chown=app:app backend/platform-gateway/migrations ./platform-gateway/migrations
COPY --chown=app:app backend/request-notification-service/migrations ./request-notification-service/migrations
COPY --chown=app:app backend/scheduler-quota-service/migrations ./scheduler-quota-service/migrations
COPY --chown=app:app backend/storage-service/migrations ./storage-service/migrations
COPY --chown=app:app backend/usage-observability-service/migrations ./usage-observability-service/migrations
COPY --chown=app:app backend/workload-service/migrations ./workload-service/migrations
USER app:app
ENV HTTP_ADDR=:8080
ENV WEB_UI_DIR=/app/web
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/microservice"]

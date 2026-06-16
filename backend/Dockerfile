FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/microservice ./cmd/microservice

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=build /out/microservice /usr/local/bin/microservice
WORKDIR /app
COPY --chown=app:app audit-compliance-service/migrations ./audit-compliance-service/migrations
COPY --chown=app:app authorization-policy-service/migrations ./authorization-policy-service/migrations
COPY --chown=app:app ide-service/migrations ./ide-service/migrations
COPY --chown=app:app identity-service/migrations ./identity-service/migrations
COPY --chown=app:app image-registry-service/migrations ./image-registry-service/migrations
COPY --chown=app:app integration-proxy-service/migrations ./integration-proxy-service/migrations
COPY --chown=app:app k8s-control-service/migrations ./k8s-control-service/migrations
COPY --chown=app:app media-upload-service/migrations ./media-upload-service/migrations
COPY --chown=app:app org-project-service/migrations ./org-project-service/migrations
COPY --chown=app:app platform-gateway/migrations ./platform-gateway/migrations
COPY --chown=app:app request-notification-service/migrations ./request-notification-service/migrations
COPY --chown=app:app scheduler-quota-service/migrations ./scheduler-quota-service/migrations
COPY --chown=app:app storage-service/migrations ./storage-service/migrations
COPY --chown=app:app usage-observability-service/migrations ./usage-observability-service/migrations
COPY --chown=app:app workload-service/migrations ./workload-service/migrations
USER app:app
ENV HTTP_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/microservice"]

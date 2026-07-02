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
# root-owned on purpose: the runtime user only reads migrations (0644/0755).
COPY backend/migrations ./migrations
USER app:app
ENV HTTP_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/microservice"]

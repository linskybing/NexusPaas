-- Additive service-owned schema for media-upload-service.
CREATE TABLE IF NOT EXISTS media_upload_records (id UUID PRIMARY KEY, resource TEXT NOT NULL, payload JSONB NOT NULL, version INTEGER NOT NULL DEFAULT 1, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS media_upload_outbox (event_id UUID PRIMARY KEY, event_name TEXT NOT NULL, trace_id TEXT NOT NULL, schema_version INTEGER NOT NULL, payload JSONB NOT NULL, occurred_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS media_upload_inbox (consumer TEXT NOT NULL, event_id UUID NOT NULL, processed_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (consumer, event_id));

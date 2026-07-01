-- Additive service-owned schema for authorization-policy-service.
CREATE TABLE IF NOT EXISTS authorization_policy_records (id UUID PRIMARY KEY, resource TEXT NOT NULL, payload JSONB NOT NULL, version INTEGER NOT NULL DEFAULT 1, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS authorization_policy_outbox (event_id UUID PRIMARY KEY, event_name TEXT NOT NULL, trace_id TEXT NOT NULL, schema_version INTEGER NOT NULL, payload JSONB NOT NULL, occurred_at TIMESTAMPTZ NOT NULL);
CREATE TABLE IF NOT EXISTS authorization_policy_inbox (consumer TEXT NOT NULL, event_id UUID NOT NULL, processed_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (consumer, event_id));

CREATE TABLE IF NOT EXISTS authorization_proxy_services (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL,
    route_path TEXT NOT NULL,
    api_patterns JSONB NOT NULL DEFAULT '[]'::jsonb,
    actions JSONB NOT NULL DEFAULT '["view","create","update","delete"]'::jsonb,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS authorization_proxy_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS authorization_proxy_policy_rules (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES authorization_proxy_policies(id) ON DELETE CASCADE,
    service_id TEXT NOT NULL REFERENCES authorization_proxy_services(id) ON DELETE RESTRICT,
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    UNIQUE (policy_id, service_id)
);

CREATE TABLE IF NOT EXISTS authorization_proxy_policy_assignments (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES authorization_proxy_policies(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL CHECK (target_type IN ('role', 'user')),
    target_id TEXT NOT NULL,
    assigned_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (policy_id, target_type, target_id)
);

CREATE TABLE IF NOT EXISTS authorization_proxy_roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS authorization_proxy_role_users (
    id TEXT PRIMARY KEY,
    role_id TEXT NOT NULL REFERENCES authorization_proxy_roles(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    assigned_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, user_id)
);

CREATE TABLE IF NOT EXISTS authorization_permission_policies (
    id TEXT PRIMARY KEY,
    policy JSONB NOT NULL,
    sub TEXT NOT NULL,
    dom TEXT NOT NULL,
    obj TEXT NOT NULL,
    act TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sub, dom, obj, act)
);

CREATE TABLE IF NOT EXISTS authorization_permission_grouping_policies (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('project_member', 'group_role')),
    user_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    domain TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (type, user_id, role, domain)
);

CREATE TABLE IF NOT EXISTS authorization_seed_markers (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_authorization_proxy_services_sort ON authorization_proxy_services (sort_order, id);
CREATE INDEX IF NOT EXISTS idx_authorization_proxy_policy_rules_policy ON authorization_proxy_policy_rules (policy_id);
CREATE INDEX IF NOT EXISTS idx_authorization_proxy_policy_assignments_policy ON authorization_proxy_policy_assignments (policy_id);
CREATE INDEX IF NOT EXISTS idx_authorization_proxy_policy_assignments_target ON authorization_proxy_policy_assignments (target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_authorization_proxy_role_users_role ON authorization_proxy_role_users (role_id);
CREATE INDEX IF NOT EXISTS idx_authorization_proxy_role_users_user ON authorization_proxy_role_users (user_id);
CREATE INDEX IF NOT EXISTS idx_authorization_permission_policies_tuple ON authorization_permission_policies (sub, dom, obj, act);
CREATE INDEX IF NOT EXISTS idx_authorization_permission_grouping_user ON authorization_permission_grouping_policies (user_id, domain);

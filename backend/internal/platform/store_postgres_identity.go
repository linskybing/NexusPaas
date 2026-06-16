package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	identityUsersResource      = "identity-service:users"
	identityRolesResource      = "identity-service:roles"
	identitySessionsResource   = "identity-service:sessions"
	identityRefreshTokens      = "identity-service:refresh_tokens"
	identityAPITokensResource  = "identity-service:api_tokens"
	identityCaptchasResource   = "identity-service:captchas"
	identityLoginFailures      = "identity-service:login_failures"
	identityDefaultRoleID      = "RO2600004"
	identityDefaultAPITokenTTL = 90 * 24 * time.Hour
	identityDefaultCaptchaTTL  = 5 * time.Minute
	identityDefaultSessionTTL  = time.Hour
	identityDefaultRefreshTTL  = 24 * time.Hour
)

type identityPostgresResource struct {
	resource string
	table    string
	insert   func(map[string]any, string, time.Time) []identityColumnValue
	update   func(map[string]any) []identityColumnValue
}

type identityColumnValue struct {
	column string
	value  any
}

var identityPostgresResources = map[string]identityPostgresResource{
	identityUsersResource: {
		resource: identityUsersResource,
		table:    "users",
		insert:   identityUserInsertColumns,
		update:   identityUserUpdateColumns,
	},
	identityRolesResource: {
		resource: identityRolesResource,
		table:    "identity_roles",
		insert:   identityRoleInsertColumns,
		update:   identityRoleUpdateColumns,
	},
	identitySessionsResource: {
		resource: identitySessionsResource,
		table:    "sessions",
		insert:   identitySessionInsertColumns,
		update:   identitySessionUpdateColumns,
	},
	identityRefreshTokens: {
		resource: identityRefreshTokens,
		table:    "refresh_tokens",
		insert:   identityRefreshTokenInsertColumns,
		update:   identityRefreshTokenUpdateColumns,
	},
	identityAPITokensResource: {
		resource: identityAPITokensResource,
		table:    "user_api_tokens",
		insert:   identityAPITokenInsertColumns,
		update:   identityAPITokenUpdateColumns,
	},
	identityCaptchasResource: {
		resource: identityCaptchasResource,
		table:    "captchas",
		insert:   identityCaptchaInsertColumns,
		update:   identityCaptchaUpdateColumns,
	},
	identityLoginFailures: {
		resource: identityLoginFailures,
		table:    "login_failures",
		insert:   identityLoginFailureInsertColumns,
		update:   identityLoginFailureUpdateColumns,
	},
}

func identityPostgresResourceFor(resource string) (identityPostgresResource, bool) {
	spec, ok := identityPostgresResources[resource]
	return spec, ok
}

func (s *PostgresStore) createIdentityRecord(
	ctx context.Context,
	spec identityPostgresResource,
	data map[string]any,
) (contracts.Record[map[string]any], error) {
	payloadMap := cloneMap(data)
	id := asString(payloadMap["id"])
	if id == "" {
		id = newID()
		payloadMap["id"] = id
		if data != nil {
			data["id"] = id
		}
	}
	sanitizeIdentityPayload(spec, payloadMap)
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return contracts.Record[map[string]any]{}, fmt.Errorf("marshal payload: %w", err)
	}
	now := time.Now().UTC()
	columns := []string{"id", "payload"}
	args := []any{id, payload}
	for _, col := range spec.insert(payloadMap, id, now) {
		columns = append(columns, col.column)
		args = append(args, col.value)
	}
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING RETURNING id, payload, version, created_at, updated_at",
		spec.table,
		strings.Join(columns, ", "),
		postgresPlaceholders(len(columns)),
	)
	record, err := scanIdentityRecord(s.db.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contracts.Record[map[string]any]{}, CreateConflictError{Resource: spec.resource, ID: id}
		}
		return contracts.Record[map[string]any]{}, fmt.Errorf("insert identity record: %w", err)
	}
	return record, nil
}

func (s *PostgresStore) getIdentityRecord(
	ctx context.Context,
	spec identityPostgresResource,
	id string,
) (contracts.Record[map[string]any], bool) {
	query := fmt.Sprintf("SELECT id, payload, version, created_at, updated_at FROM %s WHERE id = $1", spec.table)
	record, err := scanIdentityRecord(s.db.QueryRow(ctx, query, id))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("postgres identity get failed", "resource", spec.resource, "id", id, "error", err)
		}
		return contracts.Record[map[string]any]{}, false
	}
	return record, true
}

func (s *PostgresStore) listIdentityRecords(
	ctx context.Context,
	spec identityPostgresResource,
) []contracts.Record[map[string]any] {
	query := fmt.Sprintf("SELECT id, payload, version, created_at, updated_at FROM %s ORDER BY created_at, id", spec.table)
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		slog.Error("postgres identity list failed", "resource", spec.resource, "error", err)
		return nil
	}
	defer rows.Close()
	records := []contracts.Record[map[string]any]{}
	for rows.Next() {
		record, err := scanIdentityRecordRows(rows)
		if err != nil {
			slog.Error("postgres identity list scan failed", "resource", spec.resource, "error", err)
			return records
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		slog.Error("postgres identity list rows failed", "resource", spec.resource, "error", err)
	}
	return records
}

func (s *PostgresStore) updateIdentityRecord(
	ctx context.Context,
	spec identityPostgresResource,
	id string,
	data map[string]any,
) (contracts.Record[map[string]any], bool) {
	patchMap := cloneMap(data)
	sanitizeIdentityPayload(spec, patchMap)
	patch, err := json.Marshal(patchMap)
	if err != nil {
		slog.Error("postgres identity update marshal failed", "resource", spec.resource, "id", id, "error", err)
		return contracts.Record[map[string]any]{}, false
	}
	sets := []string{"payload = payload || $2::jsonb", "version = version + 1", "updated_at = now()"}
	args := []any{id, patch}
	for _, col := range spec.update(patchMap) {
		args = append(args, col.value)
		sets = append(sets, fmt.Sprintf("%s = $%d", col.column, len(args)))
	}
	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $1 RETURNING id, payload, version, created_at, updated_at",
		spec.table,
		strings.Join(sets, ", "),
	)
	record, err := scanIdentityRecord(s.db.QueryRow(ctx, query, args...))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("postgres identity update failed", "resource", spec.resource, "id", id, "error", err)
		}
		return contracts.Record[map[string]any]{}, false
	}
	return record, true
}

func (s *PostgresStore) deleteIdentityRecord(ctx context.Context, spec identityPostgresResource, id string) bool {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", spec.table)
	tag, err := s.db.Exec(ctx, query, id)
	if err != nil {
		slog.Error("postgres identity delete failed", "resource", spec.resource, "id", id, "error", err)
		return false
	}
	return tag.RowsAffected() > 0
}

func (s *PostgresStore) nextIdentityID(spec identityPostgresResource, prefix string, base, width int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key := spec.resource + "|" + prefix
	id, err := s.allocateIdentityNextID(ctx, spec, prefix, key, base, width)
	if err != nil {
		slog.Error("postgres identity NextID failed; using fallback", "resource", spec.resource, "prefix", prefix, "error", err)
		return fmt.Sprintf("%s%d", prefix, time.Now().UTC().UnixNano())
	}
	return id
}

func (s *PostgresStore) allocateIdentityNextID(
	ctx context.Context,
	spec identityPostgresResource,
	prefix string,
	key string,
	base int,
	width int,
) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, key); err != nil {
		return "", err
	}
	maxN, err := maxExistingIdentityID(ctx, tx, spec, prefix, base)
	if err != nil {
		return "", err
	}
	maxN, err = maxCachedID(ctx, tx, key, maxN)
	if err != nil {
		return "", err
	}
	id, maxN, err := nextAvailableIdentityID(ctx, tx, spec, prefix, maxN, width)
	if err != nil {
		return "", err
	}
	if err := saveIDHighWater(ctx, tx, key, maxN); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func maxExistingIdentityID(
	ctx context.Context,
	tx postgresStoreTx,
	spec identityPostgresResource,
	prefix string,
	base int,
) (int, error) {
	maxN := base - 1
	query := fmt.Sprintf("SELECT id FROM %s WHERE id LIKE $1", spec.table)
	rows, err := tx.Query(ctx, query, prefix+"%")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var existing string
		if err := rows.Scan(&existing); err != nil {
			rows.Close()
			return 0, err
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimPrefix(existing, prefix), "%d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return maxN, nil
}

func nextAvailableIdentityID(
	ctx context.Context,
	tx postgresStoreTx,
	spec identityPostgresResource,
	prefix string,
	maxN int,
	width int,
) (string, int, error) {
	var id string
	for {
		maxN++
		if width > 0 {
			id = fmt.Sprintf("%s%0*d", prefix, width, maxN)
		} else {
			id = fmt.Sprintf("%s%d", prefix, maxN)
		}
		var exists bool
		query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1)", spec.table)
		if err := tx.QueryRow(ctx, query, id).Scan(&exists); err != nil {
			return "", 0, err
		}
		if !exists {
			break
		}
	}
	return id, maxN, nil
}

func scanIdentityRecord(row postgresRow) (contracts.Record[map[string]any], error) {
	var record contracts.Record[map[string]any]
	var raw []byte
	if err := row.Scan(&record.ID, &raw, &record.Version, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if err := json.Unmarshal(raw, &record.Data); err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if record.Data == nil {
		record.Data = map[string]any{}
	}
	if record.Data["id"] == nil && record.ID != "" {
		record.Data["id"] = record.ID
	}
	return record, nil
}

func scanIdentityRecordRows(rows postgresRows) (contracts.Record[map[string]any], error) {
	var record contracts.Record[map[string]any]
	var raw []byte
	if err := rows.Scan(&record.ID, &raw, &record.Version, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if err := json.Unmarshal(raw, &record.Data); err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if record.Data == nil {
		record.Data = map[string]any{}
	}
	if record.Data["id"] == nil && record.ID != "" {
		record.Data["id"] = record.ID
	}
	return record, nil
}

func identityUserInsertColumns(data map[string]any, id string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"username", identityTextDefault(data, id, "username", "name")},
		{"email", identityNullableText(data, "email")},
		{"full_name", identityNullableText(data, "full_name", "fullName")},
		{"password_hash", identityTextDefault(data, "", "password_hash", "passwordHash")},
		{"role", identityTextDefault(data, "user", "role")},
		{"role_id", identityTextDefault(data, identityDefaultRoleID, "role_id", "roleId")},
		{"system_role", identityIntDefault(data, 2, "system_role", "systemRole")},
		{"type", identityTextDefault(data, "origin", "type")},
		{"status", identityTextDefault(data, "offline", "status")},
	}
}

func identityUserUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"username", identityTextUpdate("username", "name")},
		{"email", identityNullableTextUpdate("email")},
		{"full_name", identityNullableTextUpdate("full_name", "fullName")},
		{"password_hash", identityTextUpdate("password_hash", "passwordHash")},
		{"role", identityTextUpdate("role")},
		{"role_id", identityTextUpdate("role_id", "roleId")},
		{"system_role", identityIntUpdate("system_role", "systemRole")},
		{"type", identityTextUpdate("type")},
		{"status", identityTextUpdate("status")},
	})
}

func identityRoleInsertColumns(data map[string]any, id string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{{"name", identityTextDefault(data, id, "name")}}
}

func identityRoleUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{{"name", identityTextUpdate("name")}})
}

func identitySessionInsertColumns(data map[string]any, id string, now time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
		{"token", identityTextDefault(data, id, "token")},
		{"expires_at", identityTimeDefault(data, now.Add(identityDefaultSessionTTL), "expires_at", "expiresAt")},
	}
}

func identitySessionUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"user_id", identityTextUpdate("user_id", "userId")},
		{"token", identityTextUpdate("token")},
		{"expires_at", identityTimeUpdate("expires_at", "expiresAt")},
	})
}

func identityRefreshTokenInsertColumns(data map[string]any, id string, now time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
		{"token", identityTextDefault(data, id, "token")},
		{"expires_at", identityTimeDefault(data, now.Add(identityDefaultRefreshTTL), "expires_at", "expiresAt")},
	}
}

func identityRefreshTokenUpdateColumns(data map[string]any) []identityColumnValue {
	return identitySessionUpdateColumns(data)
}

func identityAPITokenInsertColumns(data map[string]any, _ string, now time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
		{"name", identityTextDefault(data, "", "name")},
		{"token_hash", identityTextDefault(data, "", "token_hash", "tokenHash")},
		{"token_prefix", identityTextDefault(data, "", "token_prefix", "tokenPrefix")},
		{"expires_at", identityTimeDefault(data, now.Add(identityDefaultAPITokenTTL), "expires_at", "expiresAt")},
		{"last_used_at", identityNullableTime(data, "last_used_at", "lastUsedAt")},
		{"revoked", identityBoolDefault(data, false, "revoked")},
		{"revoked_at", identityNullableTime(data, "revoked_at", "revokedAt")},
	}
}

func identityAPITokenUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"user_id", identityTextUpdate("user_id", "userId")},
		{"name", identityTextUpdate("name")},
		{"token_hash", identityTextUpdate("token_hash", "tokenHash")},
		{"token_prefix", identityTextUpdate("token_prefix", "tokenPrefix")},
		{"expires_at", identityNullableTimeUpdate("expires_at", "expiresAt")},
		{"last_used_at", identityNullableTimeUpdate("last_used_at", "lastUsedAt")},
		{"revoked", identityBoolUpdate("revoked")},
		{"revoked_at", identityNullableTimeUpdate("revoked_at", "revokedAt")},
	})
}

func identityCaptchaInsertColumns(data map[string]any, _ string, now time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"answer_hash", identityTextDefault(data, "", "answer_hash", "answerHash")},
		{"expires_at", identityTimeDefault(data, now.Add(identityDefaultCaptchaTTL), "expires_at", "expiresAt")},
	}
}

func identityCaptchaUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"answer_hash", identityTextUpdate("answer_hash", "answerHash")},
		{"expires_at", identityTimeUpdate("expires_at", "expiresAt")},
	})
}

func identityLoginFailureInsertColumns(data map[string]any, id string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"username", identityTextDefault(data, id, "username")},
		{"ip", identityTextDefault(data, "", "ip")},
		{"failures", identityIntDefault(data, 0, "failures")},
		{"locked_until", identityNullableTime(data, "locked_until", "lockedUntil")},
	}
}

func identityLoginFailureUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"username", identityTextUpdate("username")},
		{"ip", identityTextUpdate("ip")},
		{"failures", identityIntUpdate("failures")},
		{"locked_until", identityNullableTimeUpdate("locked_until", "lockedUntil")},
	})
}

type identityColumnReader struct {
	column string
	read   func(map[string]any) (any, bool)
}

func identityColumnsFromData(data map[string]any, readers []identityColumnReader) []identityColumnValue {
	columns := []identityColumnValue{}
	for _, reader := range readers {
		if value, ok := reader.read(data); ok {
			columns = append(columns, identityColumnValue{column: reader.column, value: value})
		}
	}
	return columns
}

func identityTextDefault(data map[string]any, fallback string, keys ...string) string {
	if value, ok := identityText(data, keys...); ok && value != "" {
		return value
	}
	return fallback
}

func identityNullableText(data map[string]any, keys ...string) any {
	if value, ok := identityText(data, keys...); ok && value != "" {
		return value
	}
	return nil
}

func identityTextUpdate(keys ...string) func(map[string]any) (any, bool) {
	return func(data map[string]any) (any, bool) {
		return identityText(data, keys...)
	}
}

func identityNullableTextUpdate(keys ...string) func(map[string]any) (any, bool) {
	return func(data map[string]any) (any, bool) {
		value, ok := identityText(data, keys...)
		if !ok || value == "" {
			return nil, ok
		}
		return value, true
	}
}

func identityIntDefault(data map[string]any, fallback int, keys ...string) int {
	if value, ok := identityInt(data, keys...); ok {
		return value
	}
	return fallback
}

func identityIntUpdate(keys ...string) func(map[string]any) (any, bool) {
	return func(data map[string]any) (any, bool) {
		return identityInt(data, keys...)
	}
}

func identityBoolDefault(data map[string]any, fallback bool, keys ...string) bool {
	if value, ok := identityBool(data, keys...); ok {
		return value
	}
	return fallback
}

func identityBoolUpdate(keys ...string) func(map[string]any) (any, bool) {
	return func(data map[string]any) (any, bool) {
		return identityBool(data, keys...)
	}
}

func identityTimeDefault(data map[string]any, fallback time.Time, keys ...string) time.Time {
	if value, ok := identityTime(data, keys...); ok {
		return value
	}
	return fallback
}

func identityNullableTime(data map[string]any, keys ...string) any {
	if value, ok := identityTime(data, keys...); ok {
		return value
	}
	return nil
}

func identityTimeUpdate(keys ...string) func(map[string]any) (any, bool) {
	return func(data map[string]any) (any, bool) {
		return identityTime(data, keys...)
	}
}

func identityNullableTimeUpdate(keys ...string) func(map[string]any) (any, bool) {
	return func(data map[string]any) (any, bool) {
		if _, found := identityValue(data, keys...); !found {
			return nil, false
		}
		return identityNullableTime(data, keys...), true
	}
}

func identityText(data map[string]any, keys ...string) (string, bool) {
	value, ok := identityValue(data, keys...)
	if !ok {
		return "", false
	}
	return asString(value), true
}

func identityInt(data map[string]any, keys ...string) (int, bool) {
	value, ok := identityValue(data, keys...)
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
}

func identityBool(data map[string]any, keys ...string) (bool, bool) {
	value, ok := identityValue(data, keys...)
	if !ok {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return false, false
	}
}

func identityTime(data map[string]any, keys ...string) (time.Time, bool) {
	value, ok := identityValue(data, keys...)
	if !ok {
		return time.Time{}, false
	}
	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return time.Time{}, false
		}
		return v.UTC(), true
	case string:
		if strings.TrimSpace(v) == "" {
			return time.Time{}, false
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
		return parsed.UTC(), err == nil
	default:
		return time.Time{}, false
	}
}

func identityValue(data map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := data[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func postgresPlaceholders(count int) string {
	values := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		values = append(values, fmt.Sprintf("$%d", i))
	}
	return strings.Join(values, ", ")
}

func sanitizeIdentityPayload(spec identityPostgresResource, payload map[string]any) {
	if spec.resource == identityAPITokensResource {
		delete(payload, "token")
	}
}

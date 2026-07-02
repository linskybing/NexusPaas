package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// FieldType is the JSON type expected for a registered CRUD field.
type FieldType int

const (
	// FieldString expects a JSON string.
	FieldString FieldType = iota
	// FieldNumber expects a JSON number.
	FieldNumber
	// FieldBool expects a JSON boolean.
	FieldBool
)

// crudValidator owns boundary validation for the generic-CRUD write path:
// required-field presence and per-field JSON type checks. Extracting validation
// into its own type pulls an input-validation responsibility out of the App god
// object (finding 9) and gives finding 12 a single place for schema rules.
type crudValidator struct {
	requiredFields map[string][]string
	fieldSchemas   map[string]map[string]FieldType
}

func newCRUDValidator() *crudValidator {
	return &crudValidator{
		requiredFields: map[string][]string{},
		fieldSchemas:   map[string]map[string]FieldType{},
	}
}

func (v *crudValidator) registerRequired(resource string, fields ...string) {
	v.requiredFields[resource] = append(v.requiredFields[resource], fields...)
}

func (v *crudValidator) registerSchema(resource string, fields map[string]FieldType) {
	if v.fieldSchemas[resource] == nil {
		v.fieldSchemas[resource] = map[string]FieldType{}
	}
	for field, ftype := range fields {
		v.fieldSchemas[resource][field] = ftype
	}
}

// missingRequired returns the first required field absent from payload, or ""
// when all are present. A string field counts as missing when empty/whitespace.
func (v *crudValidator) missingRequired(resource string, payload map[string]any) string {
	for _, field := range v.requiredFields[resource] {
		value, ok := payload[field]
		if !ok {
			return field
		}
		if text, isString := value.(string); isString && strings.TrimSpace(text) == "" {
			return field
		}
	}
	return ""
}

// invalidType returns the first registered field whose present value has the
// wrong JSON type, or "" when every present registered field matches. Fields
// that are absent or unregistered are left untouched, so the check is additive
// and backward compatible.
func (v *crudValidator) invalidType(resource string, payload map[string]any) string {
	for field, ftype := range v.fieldSchemas[resource] {
		value, ok := payload[field]
		if !ok || value == nil {
			continue
		}
		if !fieldMatchesType(value, ftype) {
			return field
		}
	}
	return ""
}

func fieldMatchesType(value any, ftype FieldType) bool {
	switch ftype {
	case FieldString:
		_, ok := value.(string)
		return ok
	case FieldNumber:
		switch value.(type) {
		case float64, float32, int, int64:
			return true
		default:
			return false
		}
	case FieldBool:
		_, ok := value.(bool)
		return ok
	default:
		return true
	}
}

// RegisterRequiredFields declares fields that a generic-CRUD create for the
// given resource must include (and, for string values, must be non-empty).
func (a *App) RegisterRequiredFields(resource string, fields ...string) {
	a.crud.registerRequired(resource, fields...)
}

// RegisterFieldSchema declares the expected JSON type of generic-CRUD fields for
// the given resource. Only present, registered fields are validated.
func (a *App) RegisterFieldSchema(resource string, fields map[string]FieldType) {
	a.crud.registerSchema(resource, fields)
}

// beforeCRUDFallbackCreate is a test hook.
var beforeCRUDFallbackCreate = func(*App, *httpRequest, RouteSpec, map[string]any) {
	// The default production hook is intentionally empty; tests replace it when
	// they need to observe or pause fallback creates.
}

func (a *App) handleCRUD(r *httpRequest, route RouteSpec) (int, any) {
	resource := route.Resource
	id := pathID(r.Request, route.IDParam)
	switch r.Method {
	case http.MethodGet:
		return a.handleCRUDGet(r, resource, id)
	case http.MethodPost:
		return a.handleCRUDCreate(r, route, resource)
	case http.MethodPut, http.MethodPatch:
		return a.handleCRUDUpdate(r, route, resource, id)
	case http.MethodDelete:
		eventName := domainEventName(route, "Deleted")
		deleted, err := a.deleteRecordWithEvent(r, resource, id, func(deleted bool) contracts.Event {
			return a.newEvent(r, eventName, map[string]any{"id": id, "deleted": deleted})
		})
		if err != nil {
			return http.StatusInternalServerError, map[string]any{"message": "store delete failed"}
		}
		return http.StatusOK, map[string]any{"id": id, "deleted": deleted}
	default:
		return http.StatusOK, map[string]any{"operation": route.OperationID}
	}
}

func (a *App) handleCRUDGet(r *httpRequest, resource, id string) (int, any) {
	if id == "" {
		return http.StatusOK, a.Store.List(r.Context(), resource)
	}
	record, ok := a.Store.Get(r.Context(), resource, id)
	if !ok {
		return http.StatusNotFound, map[string]any{"id": id}
	}
	return http.StatusOK, record
}

func (a *App) handleCRUDCreate(r *httpRequest, route RouteSpec, resource string) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	if missing := a.crud.missingRequired(resource, payload); missing != "" {
		return http.StatusBadRequest, map[string]any{"message": "missing required field: " + missing}
	}
	if bad := a.crud.invalidType(resource, payload); bad != "" {
		return http.StatusBadRequest, map[string]any{"message": "invalid field type: " + bad}
	}
	eventName := domainEventName(route, "Created")
	record, err := a.createRecordWithEvent(r, resource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return a.newEvent(r, eventName, record.Data)
	})
	if err != nil {
		return createErrorResponse(err)
	}
	return http.StatusCreated, record
}

func (a *App) handleCRUDUpdate(r *httpRequest, route RouteSpec, resource, id string) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	if bad := a.crud.invalidType(resource, payload); bad != "" {
		return http.StatusBadRequest, map[string]any{"message": "invalid field type: " + bad}
	}
	if id == "" {
		id = firstNonEmpty(asString(payload["id"]), newID())
	}
	eventName := domainEventName(route, "Updated")
	record, ok, err := a.updateRecordWithEvent(r, resource, id, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return a.newEvent(r, eventName, record.Data)
	})
	if err != nil {
		return http.StatusInternalServerError, map[string]any{"message": "store update failed"}
	}
	if !ok {
		payload["id"] = id
		beforeCRUDFallbackCreate(a, r, route, payload)
		record, err = a.createRecordWithEvent(r, resource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
			return a.newEvent(r, eventName, record.Data)
		})
		if err != nil {
			return createErrorResponse(err)
		}
	}
	return http.StatusOK, record
}

func (a *App) handleCommand(r *httpRequest, route RouteSpec) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	payload["status"] = "accepted"
	payload["operation"] = route.OperationID
	payload["idempotency_key"] = r.IdempotencyKey
	record, err := a.createRecordWithEvent(r, route.Resource+":commands", payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return a.newEvent(r, domainEventName(route, "Requested"), record.Data)
	})
	if err != nil {
		return createErrorResponse(err)
	}
	return http.StatusAccepted, record
}

func (a *App) handleConfigCommit(r *httpRequest, route RouteSpec) (int, any) {
	payload, err := DecodeMapWithError(r.Request)
	if err != nil {
		return http.StatusBadRequest, map[string]any{"message": errInvalidRequestBody}
	}
	content := asString(payload["content"])
	sum := sha256.Sum256([]byte(content))
	blobID := hex.EncodeToString(sum[:])
	payload["sha256"] = blobID
	payload["immutable"] = true
	payload["committed_at"] = time.Now().UTC()
	record, err := a.createRecordWithEvent(r, route.Resource+":versions", payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return a.newEvent(r, "ConfigCommitted", map[string]any{"config_id": record.ID, "sha256": blobID})
	})
	if err != nil {
		return createErrorResponse(err)
	}
	return http.StatusCreated, record
}

func createErrorResponse(err error) (int, any) {
	if IsCreateConflict(err) {
		return http.StatusConflict, map[string]any{"message": "resource already exists"}
	}
	return http.StatusInternalServerError, map[string]any{"message": "store create failed"}
}

// CreateRecordWithEvent persists a record and its domain event in one
// transaction when the store supports it (PostgresStore), falling back to
// Create + Publish otherwise. Service handlers should use this instead of a
// Store.Create followed by a separate Events.Publish, so a crash between the two
// cannot lose the event (the transactional-outbox guarantee).
func (a *App) CreateRecordWithEvent(
	ctx context.Context,
	resource string,
	data map[string]any,
	buildEvent recordEventBuilder,
) (contracts.Record[map[string]any], error) {
	if txStore, ok := transactionalStoreFor(a.Store); ok {
		return txStore.CreateWithEvent(ctx, resource, data, buildEvent)
	}
	record, err := a.Store.Create(ctx, resource, data)
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if buildEvent != nil {
		if err := a.Events.Publish(ctx, buildEvent(record)); err != nil {
			slog.Error(eventPublishFailedLogMsg, "resource", resource, "record_id", record.ID, "error", err)
			return contracts.Record[map[string]any]{}, err
		}
	}
	return record, nil
}

// UpdateRecordWithEvent is the update counterpart to CreateRecordWithEvent.
func (a *App) UpdateRecordWithEvent(
	ctx context.Context,
	resource string,
	id string,
	data map[string]any,
	buildEvent recordEventBuilder,
) (contracts.Record[map[string]any], bool, error) {
	if txStore, ok := transactionalStoreFor(a.Store); ok {
		return txStore.UpdateWithEvent(ctx, resource, id, data, buildEvent)
	}
	record, ok := a.Store.Update(ctx, resource, id, data)
	if ok && buildEvent != nil {
		if err := a.Events.Publish(ctx, buildEvent(record)); err != nil {
			slog.Error(eventPublishFailedLogMsg, "resource", resource, "record_id", record.ID, "error", err)
			return contracts.Record[map[string]any]{}, false, err
		}
	}
	return record, ok, nil
}

// UpsertRecordWithEvent persists a record by id and emits its event atomically
// when the store supports transactional upsert, falling back to update/create
// plus publish otherwise.
func (a *App) UpsertRecordWithEvent(
	ctx context.Context,
	resource string,
	id string,
	data map[string]any,
	buildEvent recordEventBuilder,
) (contracts.Record[map[string]any], error) {
	payload := cloneMap(data)
	if id == "" {
		id = asString(payload["id"])
	}
	if id == "" {
		id = newID()
	}
	payload["id"] = id
	if txStore, ok := transactionalUpsertStoreFor(a.Store); ok {
		return txStore.UpsertWithEvent(ctx, resource, id, payload, buildEvent)
	}
	record, err := a.upsertRecordWithoutTransactionalStore(ctx, resource, id, payload)
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	if buildEvent != nil {
		if err := a.Events.Publish(ctx, buildEvent(record)); err != nil {
			slog.Error(eventPublishFailedLogMsg, "resource", resource, "record_id", record.ID, "error", err)
			return contracts.Record[map[string]any]{}, err
		}
	}
	return record, nil
}

func (a *App) upsertRecordWithoutTransactionalStore(
	ctx context.Context,
	resource string,
	id string,
	payload map[string]any,
) (contracts.Record[map[string]any], error) {
	if record, ok := a.Store.Update(ctx, resource, id, payload); ok {
		return record, nil
	}
	record, err := a.Store.Create(ctx, resource, payload)
	if err == nil {
		return record, nil
	}
	if !IsCreateConflict(err) {
		return contracts.Record[map[string]any]{}, err
	}
	if updated, ok := a.Store.Update(ctx, resource, id, payload); ok {
		return updated, nil
	}
	return contracts.Record[map[string]any]{}, err
}

// DeleteRecordWithEvent is the delete counterpart to CreateRecordWithEvent.
func (a *App) DeleteRecordWithEvent(
	ctx context.Context,
	resource string,
	id string,
	buildEvent deleteEventBuilder,
) (bool, error) {
	if txStore, ok := transactionalStoreFor(a.Store); ok {
		return txStore.DeleteWithEvent(ctx, resource, id, buildEvent)
	}
	deleted := a.Store.Delete(ctx, resource, id)
	if deleted && buildEvent != nil {
		if err := a.Events.Publish(ctx, buildEvent(deleted)); err != nil {
			slog.Error(eventPublishFailedLogMsg, "resource", resource, "record_id", id, "error", err)
			return false, err
		}
	}
	return deleted, nil
}

func (a *App) createRecordWithEvent(r *httpRequest, resource string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], error) {
	return a.CreateRecordWithEvent(r.Context(), resource, data, buildEvent)
}

func (a *App) updateRecordWithEvent(r *httpRequest, resource, id string, data map[string]any, buildEvent recordEventBuilder) (contracts.Record[map[string]any], bool, error) {
	return a.UpdateRecordWithEvent(r.Context(), resource, id, data, buildEvent)
}

func (a *App) deleteRecordWithEvent(r *httpRequest, resource, id string, buildEvent deleteEventBuilder) (bool, error) {
	return a.DeleteRecordWithEvent(r.Context(), resource, id, buildEvent)
}

func transactionalStoreFor(store RecordStore) (transactionalRecordStore, bool) {
	if wrapped, ok := store.(*crossServiceStore); ok {
		return transactionalStoreFor(wrapped.local)
	}
	txStore, ok := store.(transactionalRecordStore)
	return txStore, ok
}

func transactionalUpsertStoreFor(store RecordStore) (transactionalUpserter, bool) {
	if wrapped, ok := store.(*crossServiceStore); ok {
		return transactionalUpsertStoreFor(wrapped.local)
	}
	txStore, ok := store.(transactionalUpserter)
	return txStore, ok
}

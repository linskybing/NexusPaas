package orgproject

import (
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type identityProjectionSpec struct {
	resource string
	deleted  bool
	ok       bool
}

func syncOrgIdentity(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), identityConsumer, func(event contracts.Event) error {
		return applyOrgIdentityEvent(app, r, event)
	})
}

func applyOrgIdentityEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	spec := orgIdentitySpec(event.Name)
	if !spec.ok {
		return nil
	}
	data := orgIdentityEventData(event)
	if spec.deleted {
		if deleted, ok := data["deleted"].(bool); ok && !deleted {
			return nil
		}
		app.Store.Delete(r.Context(), spec.resource, orgIdentityID(spec.resource, data))
		return nil
	}
	return saveOrgIdentity(app, r, spec.resource, data)
}

func orgIdentitySpec(name string) identityProjectionSpec {
	switch strings.ToLower(name) {
	case "usercreated", "userupdated", "userdisabled":
		return identityProjectionSpec{resource: orgIdentityUsers, ok: true}
	case "userdeleted":
		return identityProjectionSpec{resource: orgIdentityUsers, deleted: true, ok: true}
	case "rolecreated", "roleupdated":
		return identityProjectionSpec{resource: orgIdentityRoles, ok: true}
	case "roledeleted":
		return identityProjectionSpec{resource: orgIdentityRoles, deleted: true, ok: true}
	default:
		return identityProjectionSpec{}
	}
}

func orgIdentityEventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return maps.Clone(next)
	}
	return maps.Clone(event.Data)
}

func saveOrgIdentity(app *platform.App, r *http.Request, resource string, data map[string]any) error {
	id := orgIdentityID(resource, data)
	if id == "" {
		return nil
	}
	data["id"] = id
	if _, ok := app.Store.Update(r.Context(), resource, id, data); ok {
		return nil
	}
	if _, err := app.Store.Create(r.Context(), resource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := app.Store.Update(r.Context(), resource, id, data); !ok {
				return fmt.Errorf("org project identity projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("org project identity projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func orgIdentityID(resource string, data map[string]any) string {
	id := shared.TextValue(data, "id", "ID")
	userID := shared.TextValue(data, "user_id", "userId", "UserID")
	name := shared.TextValue(data, "name", "Name")
	if resource == orgIdentityRoles {
		return shared.FirstNonEmpty(id, name, userID)
	}
	return shared.FirstNonEmpty(id, userID, name)
}

func orgIdentityRows(app *platform.App, r *http.Request, localResource, sourceResource string) []map[string]any {
	syncOrgIdentity(app, r)
	local := orgRecordMaps(app, r, localResource)
	if !orgIdentitySourceCoHosted(app, sourceResource) {
		return local
	}
	source := orgRecordMaps(app, r, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergeOrgIdentityRows(localResource, source, local)
}

func orgRecordMaps(app *platform.App, r *http.Request, resource string) []map[string]any {
	if app == nil || app.Store == nil {
		return nil
	}
	records := app.Store.List(r.Context(), resource)
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if row["id"] == nil {
			row["id"] = record.ID
		}
		rows = append(rows, row)
	}
	return rows
}

func mergeOrgIdentityRows(resource string, source, local []map[string]any) []map[string]any {
	rows := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := orgIdentityID(resource, record); id != "" {
			seen[id] = true
		}
		rows = append(rows, record)
	}
	for _, record := range source {
		id := orgIdentityID(resource, record)
		if id == "" || !seen[id] {
			rows = append(rows, record)
		}
	}
	return rows
}

func orgIdentitySourceCoHosted(app *platform.App, sourceResource string) bool {
	if app == nil {
		return false
	}
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && app.Config.AllowsService(owner)
}

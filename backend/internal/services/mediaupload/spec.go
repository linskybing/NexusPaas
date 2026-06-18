package mediaupload

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id := shared.Route, shared.ID
	return platform.ServiceSpec{
		Name:        "media-upload-service",
		Category:    "support",
		Phase:       "1",
		Description: "Image uploads, JWT-only image serving, MinIO bucket abstraction, checksum, and owner references.",
		Tables:      []string{"uploaded_media", "outbox", "inbox"},
		Events:      []string{"MediaUploaded", "MediaDeleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodPost, "/api/v1/uploads/images", "uploaded_media", "create"),
			route(http.MethodGet, "/api/v1/uploads/images/{key...}", "uploaded_media", "get", id("key")),
		},
	}
}

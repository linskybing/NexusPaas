package ideworkspace

import (
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesLocalReadModels(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("IDE workspace should use local event-fed read models: %v", err)
	}
}

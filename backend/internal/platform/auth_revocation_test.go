package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type erroringRevocations struct{}

func (erroringRevocations) Revoke(context.Context, string, string, time.Duration) error {
	return errors.New("revocation store down")
}

func (erroringRevocations) IsRevoked(context.Context, string, string) (bool, error) {
	return false, errors.New("revocation store down")
}

// P0-7: a revocation-store outage must not silently let a revoked token through
// in production. Production fails closed; non-production fails open.
func TestTokenRevokedFailClosedByEnvironment(t *testing.T) {
	store := erroringRevocations{}

	prod := &App{Config: Config{Production: true}, Revocations: store}
	if !prod.tokenRevoked(context.Background(), "jwt", "jti-1") {
		t.Fatal("production must fail closed (treat as revoked) when the revocation store errors")
	}

	dev := &App{Config: Config{}, Revocations: store}
	if dev.tokenRevoked(context.Background(), "jwt", "jti-1") {
		t.Fatal("non-production must fail open when the revocation store errors")
	}
}

// P0-7 / P0-9: production must refuse to start with image checks, provenance
// enforcement, or telemetry disabled.
func TestProductionSupplyChainFailClosed(t *testing.T) {
	base := func() Config {
		return Config{
			ImageCheckEnabled:       true,
			ImageProvenanceRequired: true,
			OTLPEndpoint:            "http://collector:4318",
		}
	}
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"image check disabled", func(c *Config) { c.ImageCheckEnabled = false }, "K8S_IMAGE_CHECK_ENABLED"},
		{"provenance disabled", func(c *Config) { c.ImageProvenanceRequired = false }, "IMAGE_PUBLISH_REQUIRE_PROVENANCE"},
		{"otel missing", func(c *Config) { c.OTLPEndpoint = " " }, "OTEL_EXPORTER_OTLP_ENDPOINT"},
		{"all present", func(*Config) {}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			err := c.validateProductionSupplyChain()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("validateProductionSupplyChain() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateProductionSupplyChain() = %v, want containing %s", err, tc.want)
			}
		})
	}
}

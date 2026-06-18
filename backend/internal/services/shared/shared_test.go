package shared

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

type stringer struct{ v string }

func (s stringer) String() string { return s.v }

func TestTextValue(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		keys []string
		want string
	}{
		{"first non-empty trimmed", map[string]any{"a": "  hello  "}, []string{"a"}, "hello"},
		{"skip empty fall through", map[string]any{"a": "   ", "b": "x"}, []string{"a", "b"}, "x"},
		{"stringer", map[string]any{"a": stringer{" sv "}}, []string{"a"}, "sv"},
		{"missing", map[string]any{}, []string{"a"}, ""},
		{"non-string non-stringer", map[string]any{"a": 5}, []string{"a"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TextValue(tt.data, tt.keys...); got != tt.want {
				t.Fatalf("TextValue=%q want %q", got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "", "x", "y"); got != "x" {
		t.Fatalf("FirstNonEmpty=%q want x", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Fatalf("FirstNonEmpty=%q want empty", got)
	}
}

func TestIntValue(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		want int
	}{
		{"int", map[string]any{"n": 3}, 3},
		{"int64", map[string]any{"n": int64(4)}, 4},
		{"float64", map[string]any{"n": 5.9}, 5},
		{"json.Number", map[string]any{"n": json.Number("7")}, 7},
		{"missing", map[string]any{}, 0},
		{"string ignored", map[string]any{"n": "9"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IntValue(tt.data, "n"); got != tt.want {
				t.Fatalf("IntValue=%d want %d", got, tt.want)
			}
		})
	}
}

func TestNumberValue(t *testing.T) {
	if got := NumberValue(map[string]any{"n": 2.5}, "n"); got != 2.5 {
		t.Fatalf("NumberValue=%v want 2.5", got)
	}
	if got := NumberValue(map[string]any{"n": 4}, "n"); got != 4 {
		t.Fatalf("NumberValue=%v want 4", got)
	}
	if got := NumberValue(map[string]any{}, "n"); got != 0 {
		t.Fatalf("NumberValue=%v want 0", got)
	}
}

func TestBoolValue(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		want bool
	}{
		{"bool true", map[string]any{"b": true}, true},
		{"bool false", map[string]any{"b": false}, false},
		{"string true", map[string]any{"b": " TRUE "}, true},
		{"string other", map[string]any{"b": "nope"}, false},
		{"missing", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BoolValue(tt.data, "b"); got != tt.want {
				t.Fatalf("BoolValue=%v want %v", got, tt.want)
			}
		})
	}
}

func TestStringSlice(t *testing.T) {
	if got := StringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("StringSlice []string=%v", got)
	}
	got := StringSlice([]any{" a ", "", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("StringSlice []any=%v", got)
	}
	if StringSlice(42) != nil {
		t.Fatalf("StringSlice(non-slice) want nil")
	}
}

func TestCloneMap(t *testing.T) {
	if got := CloneMap(nil); got == nil || len(got) != 0 {
		t.Fatalf("CloneMap(nil)=%v want empty non-nil", got)
	}
	src := map[string]any{"a": 1}
	got := CloneMap(src)
	got["a"] = 2
	if src["a"] != 1 {
		t.Fatalf("CloneMap mutated source")
	}
}

func TestRouteSpecHelpers(t *testing.T) {
	spec := Route(http.MethodPost, "/internal/demo/{id}", "demo", "create", ID("id"), Admin(), ServiceInternal(), Adapter("k8s"))
	if spec.Method != http.MethodPost || spec.Pattern != "/internal/demo/{id}" || spec.Resource != "demo" || spec.Action != "create" {
		t.Fatalf("Route() = %#v, want basic fields", spec)
	}
	if spec.IDParam != "id" || !spec.Admin || !spec.PolicyBypass || spec.AuthRequired || spec.ExternalAdapter != "k8s" || !spec.StateChanging {
		t.Fatalf("Route() options = %#v, want id/admin/internal/adapter/state-changing", spec)
	}
	public := Public(Route(http.MethodGet, "/public", "public", "list", PolicyBypass()))
	if public.AuthRequired || !public.PolicyBypass || public.StateChanging {
		t.Fatalf("Public(Route()) = %#v, want public read route with policy bypass", public)
	}
}

func TestMapValue(t *testing.T) {
	if got := MapValue(map[string]any{"m": map[string]any{"x": 1}}, "m"); got["x"] != 1 {
		t.Fatalf("MapValue map case=%v", got)
	}
	if got := MapValue(map[string]any{"m": `{"x":1}`}, "m"); got["x"] != float64(1) {
		t.Fatalf("MapValue json string case=%v", got)
	}
	if got := MapValue(map[string]any{}, "m"); got == nil || len(got) != 0 {
		t.Fatalf("MapValue default=%v want empty", got)
	}
}

func TestRecordID(t *testing.T) {
	if got := RecordID(contracts.Record[map[string]any]{ID: "R1"}); got != "R1" {
		t.Fatalf("RecordID id=%q", got)
	}
	if got := RecordID(contracts.Record[map[string]any]{Data: map[string]any{"id": "R2"}}); got != "R2" {
		t.Fatalf("RecordID data fallback=%q", got)
	}
}

func TestErrorData(t *testing.T) {
	if got := ErrorData("boom"); got["message"] != "boom" {
		t.Fatalf("ErrorData=%v", got)
	}
}

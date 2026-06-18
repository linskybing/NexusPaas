package mediaupload

import "testing"

func TestSpec(t *testing.T) {
	spec := Spec()
	if spec.Name != "media-upload-service" || len(spec.Routes) == 0 {
		t.Fatalf("Spec() = %#v, want media-upload-service routes", spec)
	}
}

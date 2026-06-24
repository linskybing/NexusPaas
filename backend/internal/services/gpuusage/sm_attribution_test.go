package gpuusage

import "testing"

// GPU-018: SM attribution is only "measured" for a true per-process source;
// per-GPU sources (e.g. DCGM rollups) and unknown/empty sources are reported as
// "allocation-based" so estimated SM is never labelled as measured.
func TestSMAttributionLabelsAllocationBasedUnlessPerProcess(t *testing.T) {
	cases := map[string]string{
		"":                       "allocation-based",
		"dcgm-rollup":            "allocation-based",
		"nvml":                   "allocation-based",
		"per-process-accounting": "measured",
		"nvml_per_process":       "measured",
	}
	for source, want := range cases {
		if got := smAttribution(source); got != want {
			t.Fatalf("smAttribution(%q) = %q, want %q", source, got, want)
		}
	}
}

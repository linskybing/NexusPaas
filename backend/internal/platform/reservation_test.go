package platform

import "testing"

func TestReservationStateMachineTransitionAllowed(t *testing.T) {
	cases := []struct {
		current   string
		requested string
		want      bool
	}{
		{"reserved", "committed", true},
		{"reserved", "released", true},
		{"reserved", "reserved", false},
		{"committed", "released", true},
		{"committed", "reserved", false},
		{"committed", "committed", false},
		{"released", "committed", false},
		{"released", "released", false},
		{"", "committed", false},
		{"unknown", "released", false},
	}
	for _, tc := range cases {
		if got := reservationFSM.transitionAllowed(tc.current, tc.requested); got != tc.want {
			t.Errorf("transitionAllowed(%q,%q) = %v, want %v", tc.current, tc.requested, got, tc.want)
		}
	}
}

func TestReservationStateMachineEventName(t *testing.T) {
	cases := map[string]string{
		"reserved":  "QuotaReserved",
		"committed": "QuotaCommitted",
		"released":  "QuotaReleased",
		"":          "",
		"unknown":   "",
	}
	for state, want := range cases {
		if got := reservationFSM.eventName(state); got != want {
			t.Errorf("eventName(%q) = %q, want %q", state, got, want)
		}
	}
}

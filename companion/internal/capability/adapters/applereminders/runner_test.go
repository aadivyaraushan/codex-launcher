package applereminders

import (
	"context"
	"errors"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
)

// classifyOsascriptFailure is what Run consults after osascript exits
// non-zero. This exercises it directly, without shelling out to osascript,
// so the test is fast and does not depend on Reminders.app being reachable
// wherever it runs.
func TestClassifyOsascriptFailure(t *testing.T) {
	cause := errors.New("osascript create_reminder: boom")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name        string
		ctx         context.Context
		scriptName  string
		wantUnknown bool
	}{
		{
			name:        "creating a reminder, killed by our own cancellation, is unknown",
			ctx:         cancelled,
			scriptName:  ScriptCreateReminder,
			wantUnknown: true,
		},
		{
			name:        "creating a reminder that osascript itself reported failing is a plain failure",
			ctx:         context.Background(),
			scriptName:  ScriptCreateReminder,
			wantUnknown: false,
		},
		{
			name:        "creating the list, killed by our own cancellation, is still a plain failure (the caller always re-checks existence first)",
			ctx:         cancelled,
			scriptName:  ScriptCreateList,
			wantUnknown: false,
		},
		{
			name:        "reading reminders, killed by our own cancellation, is still a plain failure (a read changes nothing)",
			ctx:         cancelled,
			scriptName:  ScriptReadReminders,
			wantUnknown: false,
		},
		{
			name:        "checking whether the list exists, killed by our own cancellation, is still a plain failure (a read changes nothing)",
			ctx:         cancelled,
			scriptName:  ScriptListExists,
			wantUnknown: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyOsascriptFailure(tc.ctx, tc.scriptName, cause)

			var unknown *capabilityadapter.OutcomeUnknownError
			got := errors.As(err, &unknown)
			if got != tc.wantUnknown {
				t.Fatalf("errors.As(outcome unknown) = %v, want %v (err=%v)", got, tc.wantUnknown, err)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("classified error lost its cause: %v", err)
			}
			if tc.wantUnknown && unknown.AdapterID != ID {
				t.Fatalf("adapter id = %q, want %q", unknown.AdapterID, ID)
			}
		})
	}
}

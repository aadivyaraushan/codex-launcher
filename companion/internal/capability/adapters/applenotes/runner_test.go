package applenotes

import (
	"context"
	"errors"
	"testing"

	capabilityadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
)

// classifyOsascriptFailure is what Run consults after osascript exits
// non-zero. This exercises it directly, without shelling out to osascript,
// so the test is fast and does not depend on Notes.app being reachable
// wherever it runs.
func TestClassifyOsascriptFailure(t *testing.T) {
	cause := errors.New("osascript create_note: boom")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name        string
		ctx         context.Context
		scriptName  string
		wantUnknown bool
	}{
		{
			name:        "creating a note, killed by our own cancellation, is unknown",
			ctx:         cancelled,
			scriptName:  ScriptCreateNote,
			wantUnknown: true,
		},
		{
			name:        "creating a note that osascript itself reported failing is a plain failure",
			ctx:         context.Background(),
			scriptName:  ScriptCreateNote,
			wantUnknown: false,
		},
		{
			name:        "creating the folder, killed by our own cancellation, is still a plain failure (the caller always re-checks existence first)",
			ctx:         cancelled,
			scriptName:  ScriptCreateFolder,
			wantUnknown: false,
		},
		{
			name:        "reading notes, killed by our own cancellation, is still a plain failure (a read changes nothing)",
			ctx:         cancelled,
			scriptName:  ScriptReadNotes,
			wantUnknown: false,
		},
		{
			name:        "checking whether the folder exists, killed by our own cancellation, is still a plain failure (a read changes nothing)",
			ctx:         cancelled,
			scriptName:  ScriptFolderExists,
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

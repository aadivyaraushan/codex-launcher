package beeperown_test

import (
	"strings"
	"testing"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/beeperown"
)

func TestProveRejectsForeignListener(t *testing.T) {
	fs := beeperown.MemoryFS{
		ProcNetTCP: "  0: 0100007F:5B4D 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 999 1 00000000 100 0 0 10 0\n",
		Procs: map[string]beeperown.ProcSnapshot{
			"42": {
				Stat: "42 (beeper-server) S 1 42 42 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 1 0 100 0 0 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n",
				Exe:  "/usr/local/bin/beeper-server",
				SHA256: "abc",
				FDs:  map[string]string{"3": "socket:[111]"},
			},
		},
	}
	err := beeperown.Prove(fs, beeperown.Expected{
		ChildPID: 42,
		StartTime: 100,
		ExePath: "/usr/local/bin/beeper-server",
		ExeSHA256: "abc",
		Port: 23373,
	})
	if err == nil || !strings.Contains(err.Error(), "listener") {
		t.Fatalf("want foreign listener rejection, got %v", err)
	}
}

func TestProveAcceptsOwnedLoopbackListener(t *testing.T) {
	fs := beeperown.MemoryFS{
		ProcNetTCP: "  0: 0100007F:5B4D 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 777 1 00000000 100 0 0 10 0\n",
		Procs: map[string]beeperown.ProcSnapshot{
			"42": {
				Stat: "42 (beeper-server) S 1 42 42 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 1 0 100 0 0 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n",
				Exe:  "/usr/local/bin/beeper-server",
				SHA256: "abc",
				FDs:  map[string]string{"3": "socket:[777]"},
			},
		},
	}
	if err := beeperown.Prove(fs, beeperown.Expected{
		ChildPID: 42,
		StartTime: 100,
		ExePath: "/usr/local/bin/beeper-server",
		ExeSHA256: "abc",
		Port: 23373,
	}); err != nil {
		t.Fatalf("Prove: %v", err)
	}
}

func TestProveRejectsStartTimeReuse(t *testing.T) {
	fs := beeperown.MemoryFS{
		ProcNetTCP: "  0: 0100007F:5B4D 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 777 1 00000000 100 0 0 10 0\n",
		Procs: map[string]beeperown.ProcSnapshot{
			"42": {
				Stat: "42 (beeper-server) S 1 42 42 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 1 0 999 0 0 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n",
				Exe:  "/usr/local/bin/beeper-server",
				SHA256: "abc",
				FDs:  map[string]string{"3": "socket:[777]"},
			},
		},
	}
	err := beeperown.Prove(fs, beeperown.Expected{
		ChildPID: 42,
		StartTime: 100,
		ExePath: "/usr/local/bin/beeper-server",
		ExeSHA256: "abc",
		Port: 23373,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "start") {
		t.Fatalf("want start-time rejection, got %v", err)
	}
}

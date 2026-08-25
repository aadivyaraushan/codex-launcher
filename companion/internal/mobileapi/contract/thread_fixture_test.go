package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The thread fixtures are the golden frames for the capability-thread UI: a
// task_page carrying "message" transcript entries (one row per received DM,
// with sender, text, and sentAt) and the open_page device action that opens a
// specific page inside an app. Every line must decode through the production
// contract; the Kotlin suite reads the same file, so a frame accepted here but
// rejected there fails that side's build instead of dying on the phone.
func TestThreadFixturesDecodeThroughProductionContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "protocol", "fixtures", "thread.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		message, err := DecodeText([]byte(line))
		if err != nil {
			t.Fatalf("thread.jsonl:%d: %v", lineNumber+1, err)
		}
		if message.Type == "" || message.MessageID == "" {
			t.Fatalf("thread.jsonl:%d decoded empty envelope", lineNumber+1)
		}
	}
}

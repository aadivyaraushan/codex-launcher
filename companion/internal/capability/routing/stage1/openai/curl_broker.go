// Fact-force:
// 1) Callers: Client.Model brokered path.
// 2) Termux watcher at ~/broker-work/watch.sh (outside proot) curls Android :9451.
// 3) Phone-runtime (Debian proot) drops req.json and polls resp.http — no in-proot
//    socket to the broker (that path deadlocks while a WebSocket is open).
// 4) User: Pixel OpenAI+Beeper dogfood.
package openai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const termuxBrokerWorkDir = "/data/data/com.termux/files/home/broker-work"

func curlBrokerPOST(ctx context.Context, baseURL, path string, body []byte, timeout time.Duration) (status int, respBody []byte, err error) {
	_ = baseURL
	_ = path
	dir := termuxBrokerWorkDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, nil, fmt.Errorf("broker work dir: %w", err)
	}
	reqPath := filepath.Join(dir, "req.json")
	respPath := filepath.Join(dir, "resp.http")
	_ = os.Remove(respPath)
	tmp := reqPath + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return 0, nil, err
	}
	if err := os.Rename(tmp, reqPath); err != nil {
		return 0, nil, err
	}

	deadline := time.Now().Add(timeout)
	if timeout <= 0 {
		deadline = time.Now().Add(60 * time.Second)
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		if time.Now().After(deadline) {
			return 0, nil, fmt.Errorf("broker file drop: timeout waiting for Termux watcher")
		}
		raw, readErr := os.ReadFile(respPath)
		if readErr != nil {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		_ = os.Remove(respPath)
		text := string(raw)
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			return 0, nil, fmt.Errorf("broker file drop: missing status line")
		}
		status, err = strconv.Atoi(strings.TrimSpace(text[:idx]))
		if err != nil {
			return 0, nil, fmt.Errorf("broker file drop: bad status %q", text[:idx])
		}
		return status, []byte(text[idx+1:]), nil
	}
}

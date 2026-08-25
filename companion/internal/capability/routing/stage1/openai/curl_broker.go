// Fact-force:
// 1) Callers: Client.Model brokered path.
// 2) On Pixel: Termux watcher ~/broker-work/watch.sh curls Android :9451.
//    Phone-runtime drops req.json and polls resp.http (avoids proot deadlock).
// 3) Elsewhere / unit tests: plain http.Client POST to baseURL+path.
// 4) User: Pixel OpenAI+Beeper dogfood.
package openai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const termuxBrokerWorkDir = "/data/data/com.termux/files/home/broker-work"

func curlBrokerPOST(ctx context.Context, baseURL, path string, body []byte, timeout time.Duration) (status int, respBody []byte, err error) {
	// Prefer direct HTTP to Android :9451. File-drop via Termux watcher is opt-in
	// (BROKER_FILE_DROP=1) because the watcher has been timing out on Pixel dogfood
	// while plain HTTP from debian succeeds.
	if os.Getenv("BROKER_FILE_DROP") == "1" {
		if _, err := os.Stat(filepath.Join(termuxBrokerWorkDir, "watch.sh")); err == nil {
			return fileDropBrokerPOST(ctx, body, timeout)
		}
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return httpBrokerPOST(ctx, baseURL, path, body, timeout)
}

func fileDropBrokerPOST(ctx context.Context, body []byte, timeout time.Duration) (status int, respBody []byte, err error) {
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
		deadline = time.Now().Add(120 * time.Second)
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

func httpBrokerPOST(ctx context.Context, baseURL, path string, body []byte, timeout time.Duration) (status int, respBody []byte, err error) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			ExpectContinueTimeout: 0,
			ForceAttemptHTTP2:     false,
			DisableKeepAlives:     true,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, respBody, nil
}

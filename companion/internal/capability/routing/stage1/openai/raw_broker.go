// Fact-force:
// 1) Callers: Client.Model when responsesPath is set (NewBrokered).
// 2) rg rawBrokerPOST → this file + client.go; dogfood on Termux proot.
// 3) Plain TCP HTTP/1.1 POST, no Expect: 100-continue.
// 4) User: Pixel dogfood — Go net/http hung ~90s on broker POST while WS up;
//    curl/raw dial from the same Debian finished in ~1s.
package openai

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func rawBrokerPOST(ctx context.Context, baseURL, path string, body []byte, timeout time.Duration) (status int, respBody []byte, err error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + path)
	if err != nil {
		return 0, nil, err
	}
	host := u.Host
	if host == "" {
		return 0, nil, fmt.Errorf("raw broker: missing host in %q", baseURL)
	}
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return 0, nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return 0, nil, err
	}

	var req strings.Builder
	req.WriteString("POST ")
	req.WriteString(path)
	req.WriteString(" HTTP/1.1\r\nHost: ")
	req.WriteString(host)
	req.WriteString("\r\nContent-Type: application/json\r\nContent-Length: ")
	req.WriteString(strconv.Itoa(len(body)))
	req.WriteString("\r\nConnection: close\r\n\r\n")
	header := req.String()
	if _, err := io.WriteString(conn, header); err != nil {
		return 0, nil, err
	}
	// proot/loopback often returns short writes; Content-Length must match bytes
	// actually on the wire or Android blocks until the client deadline.
	written := 0
	for written < len(body) {
		n, err := conn.Write(body[written:])
		written += n
		if err != nil {
			return 0, nil, fmt.Errorf("raw broker: write body %d/%d: %w", written, len(body), err)
		}
		if n == 0 {
			return 0, nil, fmt.Errorf("raw broker: write body stalled at %d/%d", written, len(body))
		}
	}


	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, nil, err
	}
	parts := strings.Split(strings.TrimSpace(statusLine), " ")
	if len(parts) < 2 {
		return 0, nil, fmt.Errorf("raw broker: bad status line %q", statusLine)
	}
	status, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, nil, fmt.Errorf("raw broker: bad status code in %q", statusLine)
	}
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			contentLength, _ = strconv.Atoi(strings.TrimSpace(line[len("content-length:"):]))
		}
	}
	if contentLength < 0 {
		respBody, err = io.ReadAll(io.LimitReader(reader, 1<<20))
		return status, respBody, err
	}
	respBody = make([]byte, contentLength)
	_, err = io.ReadFull(reader, respBody)
	return status, respBody, err
}

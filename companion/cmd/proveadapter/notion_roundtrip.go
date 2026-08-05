// Callers: runNotion in notion_oauth.go after Connect measure succeeds.
// Affected API: Notion MCP create-pages / search / fetch under Operator container.
// Schemas: none durable beyond Keychain oauth record already stored.
// User: "Run live Notion read/write proof per plan with durable evidence."
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/adapter"
	notionadapter "github.com/codex-launcher/codex-launcher/companion/internal/capability/adapters/notion"
	"github.com/codex-launcher/codex-launcher/companion/internal/capability/manifest"
)

func proveNotionReadWrite(ctx context.Context, accessToken string) error {
	step(4, "Live Notion write under Operator container, then search+fetch marker")
	session := notionadapter.NewHTTPSession(nil, notionadapter.ServerURL, accessToken)
	a, err := notionadapter.New(session)
	if err != nil {
		return err
	}
	ceiling, err := a.Connect(ctx)
	if err != nil {
		return fmt.Errorf("notion roundtrip connect: %w", err)
	}
	line("tools measured; ceiling=%s", ceiling)

	marker := fmt.Sprintf("OP-NOTION-%s", time.Now().UTC().Format("20060102T150405Z"))
	writePlan, err := a.Resolve(ctx, adapter.Intent{
		AdapterID: notionadapter.ID, Verb: manifest.Write, Subject: marker, Body: marker + " live proof",
	})
	if err != nil {
		return fmt.Errorf("notion roundtrip resolve write: %w", err)
	}
	if _, err := a.Preview(ctx, writePlan); err != nil {
		return fmt.Errorf("notion roundtrip preview write: %w", err)
	}
	writeOut, err := a.Execute(ctx, writePlan)
	if err != nil {
		return fmt.Errorf("notion roundtrip execute write: %w", err)
	}
	if !writeOut.Done {
		return fmt.Errorf("notion roundtrip write not done: detail=%q", writeOut.Detail)
	}
	pageID := pageIDFromWriteDetail(writeOut.Detail)
	if pageID == "" {
		return fmt.Errorf("notion roundtrip write detail missing page_id: %q", writeOut.Detail)
	}
	line("write done under Operator; marker=%s page_id=%s", marker, pageID)

	// Fetch by the created page id — Notion search can lag right after create.
	readPlan := adapter.Plan{
		AdapterID: notionadapter.ID,
		Verb:      manifest.Read,
		Handle:    pageID,
		Summary:   fmt.Sprintf("Read %q", marker),
		Details:   map[string]string{"page_id": pageID, "title": marker},
	}
	readOut, err := a.Execute(ctx, readPlan)
	if err != nil {
		return fmt.Errorf("notion roundtrip execute read: %w", err)
	}
	if !readOut.Done {
		return fmt.Errorf("notion roundtrip read not done: detail=%q", readOut.Detail)
	}
	if !strings.Contains(readOut.Detail, marker) {
		return fmt.Errorf("notion roundtrip read detail missing marker %q (got %q)", marker, truncateForLog(readOut.Detail, 160))
	}
	line("read matched marker; page_id_present=true")
	verdict("Notion live write+read under Operator container proven (marker matched)")
	return nil
}

func pageIDFromWriteDetail(detail string) string {
	const key = "page_id="
	i := strings.Index(detail, key)
	if i < 0 {
		return ""
	}
	id := strings.TrimSpace(detail[i+len(key):])
	if j := strings.IndexAny(id, " \t\n"); j >= 0 {
		id = id[:j]
	}
	return id
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

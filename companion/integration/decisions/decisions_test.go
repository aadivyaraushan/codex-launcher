package decisions_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

func TestLiveCodexApprovalRoutesThroughExactPrivateRequestID(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	t.Cleanup(func() { _ = clientSide.Close(); _ = serverSide.Close() })
	wireResponse := make(chan map[string]json.RawMessage, 1)
	go func() {
		reader := bufio.NewReader(serverSide)
		encoder := json.NewEncoder(serverSide)
		line, _ := reader.ReadBytes('\n')
		var initialize map[string]json.RawMessage
		_ = json.Unmarshal(line, &initialize)
		_ = encoder.Encode(map[string]any{"id": initialize["id"], "result": map[string]any{"codexHome": "/tmp/codex", "platformFamily": "unix", "platformOs": "macos", "userAgent": "integration"}})
		_, _ = reader.ReadBytes('\n')
		_ = encoder.Encode(map[string]any{"id": 17, "method": "item/commandExecution/requestApproval", "params": map[string]any{
			"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-1", "startedAtMs": 1, "command": "npm test", "availableDecisions": []string{"decline"},
		}})
		line, _ = reader.ReadBytes('\n')
		var response map[string]json.RawMessage
		_ = json.Unmarshal(line, &response)
		wireResponse <- response
	}()

	client := appserver.NewClient(clientSide, clientSide, slog.New(slog.NewTextHandler(io.Discard, nil)), appserver.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	owner := decisions.NewAppServerOwner(client)
	router := decisions.NewRouter(owner, nil)
	source := <-client.Requests()
	request, err := owner.Register(source, decisions.DisplayContext{ComputerName: "Aadi Mac", ProjectLabel: "Launcher"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := router.Add(request); err != nil {
		t.Fatal(err)
	}
	if err := router.Respond(ctx, decisions.Response{
		RequestID: request.ID, ThreadID: request.ThreadID, TurnID: request.TurnID, ItemID: request.ItemID, Kind: request.Kind, Decision: decisions.DecisionDecline,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := <-wireResponse
	if string(response["id"]) != "17" || string(response["result"]) != `{"decision":"decline"}` {
		t.Fatalf("wire response = %#v", response)
	}
}

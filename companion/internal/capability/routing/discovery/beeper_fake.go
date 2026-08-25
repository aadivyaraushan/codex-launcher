package discovery

import (
	"context"

	"github.com/codex-launcher/codex-launcher/companion/internal/capability/messaging/beeper"
)

// NoOpBeeperAPI satisfies beepermessage.API with no I/O. It exists only so
// capabilityruntime.NewProduction sees config.BeeperAPI != nil and
// registers the beeper_messaging class the way a live Beeper connection
// would — a routing sweep only resolves Decisions, it never calls
// Resolve/Execute on the adapter, so nothing here needs to do real work.
type NoOpBeeperAPI struct{}

func (NoOpBeeperAPI) SearchChats(context.Context, string) ([]beeper.Chat, error) {
	return nil, nil
}

func (NoOpBeeperAPI) Accounts(context.Context) ([]beeper.Account, error) {
	return nil, nil
}

func (NoOpBeeperAPI) StartChat(context.Context, string, string) (beeper.Chat, error) {
	return beeper.Chat{}, nil
}

func (NoOpBeeperAPI) Send(context.Context, string, string) (beeper.Sent, error) {
	return beeper.Sent{}, nil
}

// The read and manage methods below exist for the same reason as the four
// above: a routing sweep only resolves Decisions, so the beeper_messaging
// adapter's widened API surface is satisfied here with no I/O.

func (NoOpBeeperAPI) ListChats(context.Context, beeper.ListChatsOptions) ([]beeper.Chat, error) {
	return nil, nil
}

func (NoOpBeeperAPI) ListMessages(context.Context, string, int) ([]beeper.Message, error) {
	return nil, nil
}

func (NoOpBeeperAPI) Reply(context.Context, string, string, string) (beeper.Sent, error) {
	return beeper.Sent{}, nil
}

func (NoOpBeeperAPI) EditMessage(context.Context, string, string, string) error {
	return nil
}

func (NoOpBeeperAPI) DeleteMessage(context.Context, string, string) error {
	return nil
}

func (NoOpBeeperAPI) React(context.Context, string, string, string) error {
	return nil
}

func (NoOpBeeperAPI) Unreact(context.Context, string, string, string) error {
	return nil
}

func (NoOpBeeperAPI) MarkRead(context.Context, string) error {
	return nil
}

func (NoOpBeeperAPI) MarkUnread(context.Context, string) error {
	return nil
}

func (NoOpBeeperAPI) Archive(context.Context, string, bool) error {
	return nil
}

func (NoOpBeeperAPI) UpdateChat(context.Context, string, beeper.ChatState) error {
	return nil
}

func (NoOpBeeperAPI) SetReminder(context.Context, string, string) error {
	return nil
}

func (NoOpBeeperAPI) ClearReminder(context.Context, string) error {
	return nil
}

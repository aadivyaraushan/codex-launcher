package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/device"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/store"
)

type RunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func RunOpenClaw(ctx context.Context, name string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return out, err
}

func ListOpenAI(ctx context.Context) ([]modelauth.Profile, error) {
	return listOpenAI(ctx, store.Read, RunOpenClaw)
}

func listOpenAI(ctx context.Context, read func() ([]modelauth.Profile, error), run RunFunc) ([]modelauth.Profile, error) {
	if read != nil {
		profiles, err := read()
		if err == nil {
			return profiles, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	return ListOpenAIWith(ctx, run)
}

func ListOpenAIWith(ctx context.Context, run RunFunc) ([]modelauth.Profile, error) {
	if run == nil {
		run = RunOpenClaw
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := run(ctx, "openclaw", "models", "auth", "list", "--provider", "openai", "--json")
	if err != nil {
		return nil, fmt.Errorf("openclaw models auth list: %w", err)
	}
	profiles, parseErr := modelauth.ParseAuthListJSON(extractJSONObject(out))
	if parseErr != nil {
		return nil, parseErr
	}
	return profiles, nil
}

func SetOpenAIAuthOrder(ctx context.Context, ids []string) error {
	return SetOpenAIAuthOrderWith(ctx, RunOpenClaw, ids)
}

func SetOpenAIAuthOrderWith(ctx context.Context, run RunFunc, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if run == nil {
		run = RunOpenClaw
	}
	args := append([]string{"models", "auth", "order", "set", "--provider", "openai"}, ids...)
	if _, err := run(ctx, "openclaw", args...); err != nil {
		return fmt.Errorf("openclaw models auth order set: %w", err)
	}
	return nil
}

func RestartGateway(ctx context.Context) error {
	return RestartGatewayWith(ctx, RunOpenClaw)
}

func RestartGatewayWith(ctx context.Context, run RunFunc) error {
	if run == nil {
		run = RunOpenClaw
	}
	if _, err := run(ctx, "openclaw", "gateway", "restart"); err == nil {
		return nil
	}
	if _, err := run(ctx, "sv", "restart", "openclaw-gateway"); err != nil {
		return fmt.Errorf("restart openclaw gateway: %w", err)
	}
	return nil
}

func extractJSONObject(raw []byte) []byte {
	start := bytes.IndexByte(raw, '{')
	end := bytes.LastIndexByte(raw, '}')
	if start < 0 || end < start {
		return raw
	}
	return raw[start : end+1]
}

func RedactOutput(raw []byte) string {
	return device.Redact(string(raw))
}

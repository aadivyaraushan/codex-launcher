package phoneruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/device"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/store"
)

func (runtime *Runtime) wireModelAuth(hooks ModelAuthHooks) {
	production := hooks.List == nil && hooks.StartLogin == nil && hooks.SetAuthOrder == nil && hooks.RestartGateway == nil
	if production {
		runtime.listModelAuth = cli.ListOpenAI
		runtime.startModelAuth = runtime.startOpenClawDeviceLogin
		runtime.setAuthOrder = cli.SetOpenAIAuthOrder
		runtime.restartGateway = cli.RestartGateway
		runtime.lastModelAuth = string(modelauth.Missing)
		if profiles, err := store.Read(); err == nil {
			runtime.logger.Info("[model-auth] seeded from disk store", "profile_count", len(profiles), "decision", "cache_from_store")
			runtime.applyModelAuthList(profiles, nil)
		} else if !errors.Is(err, store.ErrNotFound) {
			runtime.logger.Info("[model-auth] disk store unreadable", "error", device.Redact(err.Error()), "decision", "keep_cache")
		}
		runtime.kickModelAuthRefresh()
		return
	}
	runtime.listModelAuth = hooks.List
	runtime.startModelAuth = hooks.StartLogin
	runtime.setAuthOrder = hooks.SetAuthOrder
	runtime.restartGateway = hooks.RestartGateway
	runtime.lastModelAuth = string(modelauth.Missing)
	runtime.kickModelAuthRefresh()
}

func (runtime *Runtime) modelAuthStatus() string {
	if runtime == nil {
		return string(modelauth.Missing)
	}
	runtime.kickModelAuthRefresh()
	runtime.mu.Lock()
	status := runtime.lastModelAuth
	pending := runtime.modelAuthPending
	runtime.mu.Unlock()
	if status == "" {
		status = string(modelauth.Missing)
	}
	if pending && status != string(modelauth.OauthReady) {
		return string(modelauth.Pending)
	}
	return status
}

func (runtime *Runtime) kickModelAuthRefresh() {
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	if runtime.closed || runtime.modelAuthRefreshInFlight || runtime.listModelAuth == nil {
		runtime.mu.Unlock()
		return
	}
	runtime.modelAuthRefreshInFlight = true
	list := runtime.listModelAuth
	runtime.mu.Unlock()
	go runtime.refreshModelAuth(list)
}

func (runtime *Runtime) refreshModelAuth(list func(context.Context) ([]modelauth.Profile, error)) {
	defer func() {
		runtime.mu.Lock()
		runtime.modelAuthRefreshInFlight = false
		runtime.mu.Unlock()
	}()
	if list == nil {
		return
	}
	runtime.mu.Lock()
	closed := runtime.closed
	runtime.mu.Unlock()
	if closed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	type outcome struct {
		profiles []modelauth.Profile
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		profiles, err := list(ctx)
		done <- outcome{profiles: profiles, err: err}
	}()
	var profiles []modelauth.Profile
	var err error
	select {
	case out := <-done:
		profiles, err = out.profiles, out.err
	case <-ctx.Done():
		err = ctx.Err()
		runtime.logger.Info("[model-auth] list hung; further refresh is disk-only", "decision", "store_only")
		runtime.applyModelAuthList(nil, err)
		runtime.mu.Lock()
		runtime.modelAuthRefreshGen++
		runtime.listModelAuth = runtime.storeOnlyModelAuth
		runtime.mu.Unlock()
		cancel()
		return
	}
	cancel()
	runtime.applyModelAuthList(profiles, err)
}

func (runtime *Runtime) storeOnlyModelAuth(context.Context) ([]modelauth.Profile, error) {
	return store.Read()
}

func (runtime *Runtime) applyModelAuthList(profiles []modelauth.Profile, err error) {
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	pending := runtime.modelAuthPending
	runtime.mu.Unlock()
	if err != nil {
		runtime.mu.Lock()
		firstListErr := !runtime.modelAuthListLogged
		runtime.modelAuthListLogged = true
		keepReady := runtime.modelAuthSawOAuth || runtime.lastModelAuth == string(modelauth.OauthReady)
		if keepReady {
			runtime.lastModelAuth = string(modelauth.OauthReady)
		}
		status := runtime.lastModelAuth
		if status == "" {
			status = string(modelauth.Missing)
		}
		runtime.mu.Unlock()
		if firstListErr {
			runtime.logger.Info("[model-auth] list failed", "error", device.Redact(err.Error()), "decision", "keep_cache", "status", status)
		}
		return
	}
	status := modelauth.Classify(profiles, pending)
	runtime.mu.Lock()
	changed := runtime.lastModelAuth != string(status)
	runtime.lastModelAuth = string(status)
	if status == modelauth.OauthReady {
		runtime.modelAuthSawOAuth = true
		runtime.modelAuthPending = false
	} else {
		runtime.modelAuthSawOAuth = false
	}
	runtime.mu.Unlock()
	if changed {
		runtime.logger.Info("[model-auth] health", "status", string(status), "profile_count", len(profiles), "pending", pending, "decision", "cache_updated")
	}
	if status == modelauth.OauthReady {
		runtime.kickPreferOAuth()
	}
}

func (runtime *Runtime) kickPreferOAuth() {
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	if runtime.closed || runtime.authOrderApplied || runtime.preferOrderInFlight {
		runtime.mu.Unlock()
		return
	}
	runtime.preferOrderInFlight = true
	runtime.mu.Unlock()
	go runtime.preferOAuthAfterLogin()
}

func (runtime *Runtime) StartModelAuth(ctx context.Context) (device.Prompt, error) {
	if runtime == nil {
		return device.Prompt{}, ErrMissingDependency
	}
	runtime.mu.Lock()
	if runtime.modelAuthPending && runtime.pendingModelAuth.UserCode != "" {
		existing := runtime.pendingModelAuth
		runtime.mu.Unlock()
		runtime.logger.Info("[model-auth] start reused in-flight device code", "decision", "reuse_pending", "user_code_len", len(existing.UserCode))
		return existing, nil
	}
	start := runtime.startModelAuth
	runtime.mu.Unlock()
	if start == nil {
		return device.Prompt{}, ErrMissingDependency
	}
	runtime.logger.Info("[model-auth] start requested", "decision", "spawn_device_login")
	prompt, err := start(ctx)
	if err != nil {
		runtime.logger.Info("[model-auth] start failed", "error", device.Redact(err.Error()), "decision", "fail_closed")
		return device.Prompt{}, err
	}
	if !strings.HasPrefix(prompt.VerificationURL, "https://auth.openai.com/") || prompt.UserCode == "" {
		runtime.logger.Info("[model-auth] start rejected incomplete prompt", "decision", "fail_closed", "user_code_len", len(prompt.UserCode))
		return device.Prompt{}, ErrInvalidConfig
	}
	runtime.mu.Lock()
	runtime.modelAuthPending = true
	runtime.pendingModelAuth = prompt
	runtime.mu.Unlock()
	runtime.logger.Info("[model-auth] device code ready", "decision", "return_prompt", "user_code_len", len(prompt.UserCode), "verification_host", "auth.openai.com")
	runtime.kickPreferOAuth()
	return prompt, nil
}

func (runtime *Runtime) preferOAuthAfterLogin() {
	if runtime == nil {
		return
	}
	defer func() {
		runtime.mu.Lock()
		if !runtime.authOrderApplied {
			runtime.preferOrderInFlight = false
		}
		runtime.mu.Unlock()
	}()
	for attempt := 0; attempt < 25; attempt++ {
		runtime.mu.Lock()
		if runtime.closed {
			runtime.mu.Unlock()
			return
		}
		list := runtime.listModelAuth
		setOrder := runtime.setAuthOrder
		restart := runtime.restartGateway
		applied := runtime.authOrderApplied
		runtime.mu.Unlock()
		if list == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		profiles, err := list(ctx)
		cancel()
		if err != nil {
			runtime.logger.Info("[model-auth] list after login failed", "error", device.Redact(err.Error()))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if modelauth.Classify(profiles, false) != modelauth.OauthReady {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		runtime.applyModelAuthList(profiles, nil)
		ids := modelauth.PreferOrder(profiles)
		if applied {
			runtime.logger.Info("[model-auth] oauth ready; order already applied", "decision", "skip_restart", "oauth_first", len(ids) > 0)
			return
		}
		if setOrder != nil && len(ids) > 0 {
			orderCtx, orderCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := setOrder(orderCtx, ids); err != nil {
				runtime.logger.Info("[model-auth] auth order set failed", "error", device.Redact(err.Error()), "decision", "continue")
			} else {
				runtime.logger.Info("[model-auth] auth order prefers oauth", "decision", "order_set", "id_count", len(ids))
			}
			orderCancel()
		}
		if restart != nil {
			restartCtx, restartCancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := restart(restartCtx); err != nil {
				runtime.logger.Info("[model-auth] gateway restart failed", "error", device.Redact(err.Error()), "decision", "continue")
			} else {
				runtime.logger.Info("[model-auth] gateway restarted for oauth profile", "decision", "restarted")
			}
			restartCancel()
		}
		runtime.mu.Lock()
		runtime.authOrderApplied = true
		runtime.mu.Unlock()
		return
	}
}

func (runtime *Runtime) startOpenClawDeviceLogin(ctx context.Context) (device.Prompt, error) {
	_ = ctx
	procCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	cmd := exec.CommandContext(procCtx, "openclaw", "models", "auth", "login", "--provider", "openai", "--device-code")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return device.Prompt{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return device.Prompt{}, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return device.Prompt{}, err
	}
	reader := io.MultiReader(stdout, stderr)
	prompt, readErr := readDevicePrompt(reader, 30*time.Second)
	if readErr != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		cancel()
		return device.Prompt{}, readErr
	}
	go func() {
		defer cancel()
		waitErr := cmd.Wait()
		if waitErr != nil {
			runtime.mu.Lock()
			runtime.modelAuthPending = false
			runtime.pendingModelAuth = device.Prompt{}
			runtime.mu.Unlock()
			runtime.logger.Info("[model-auth] device login process ended", "error", device.Redact(waitErr.Error()), "decision", "process_exit")
			return
		}
		runtime.logger.Info("[model-auth] device login process succeeded", "decision", "process_ok")
		runtime.kickPreferOAuth()
	}()
	return prompt, nil
}

func readDevicePrompt(r io.Reader, wait time.Duration) (device.Prompt, error) {
	deadline := time.Now().Add(wait)
	var builder strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && err != io.EOF {
				return device.Prompt{}, err
			}
			if prompt, ok := device.ParseOutput(builder.String()); ok {
				return prompt, nil
			}
			break
		}
		builder.WriteString(scanner.Text())
		builder.WriteByte('\n')
		if prompt, ok := device.ParseOutput(builder.String()); ok {
			return prompt, nil
		}
	}
	if prompt, ok := device.ParseOutput(builder.String()); ok {
		return prompt, nil
	}
	return device.Prompt{}, ErrInvalidConfig
}

type modelAuthStartResponse struct {
	UserCode        string `json:"userCode"`
	VerificationURL string `json:"verificationUrl"`
}

func (runtime *Runtime) serveModelAuthStart(writer http.ResponseWriter, request *http.Request) {
	if !runtime.loopbackOnly(request.RemoteAddr) {
		http.Error(writer, "loopback only", http.StatusForbidden)
		runtime.logger.Info("[model-auth] start rejected", "reason", "non_loopback")
		return
	}
	prompt, err := runtime.StartModelAuth(request.Context())
	if err != nil {
		http.Error(writer, "model auth start failed", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(writer).Encode(modelAuthStartResponse{
		UserCode:        prompt.UserCode,
		VerificationURL: prompt.VerificationURL,
	})
}

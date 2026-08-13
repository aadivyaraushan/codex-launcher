package phoneruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/cli"
	"github.com/codex-launcher/codex-launcher/companion/internal/phoneruntime/modelauth/device"
)

func (runtime *Runtime) wireModelAuth(hooks ModelAuthHooks) {
	production := hooks.List == nil && hooks.StartLogin == nil && hooks.SetAuthOrder == nil && hooks.RestartGateway == nil
	if production {
		runtime.listModelAuth = cli.ListOpenAI
		runtime.startModelAuth = runtime.startOpenClawDeviceLogin
		runtime.setAuthOrder = cli.SetOpenAIAuthOrder
		runtime.restartGateway = cli.RestartGateway
		return
	}
	runtime.listModelAuth = hooks.List
	runtime.startModelAuth = hooks.StartLogin
	runtime.setAuthOrder = hooks.SetAuthOrder
	runtime.restartGateway = hooks.RestartGateway
}

func (runtime *Runtime) modelAuthStatus() string {
	if runtime == nil {
		return string(modelauth.Missing)
	}
	runtime.mu.Lock()
	pending := runtime.modelAuthPending
	list := runtime.listModelAuth
	runtime.mu.Unlock()
	var profiles []modelauth.Profile
	if list != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		got, err := list(ctx)
		cancel()
		if err != nil {
			runtime.mu.Lock()
			firstListErr := !runtime.modelAuthListLogged
			runtime.modelAuthListLogged = true
			runtime.mu.Unlock()
			if firstListErr {
				runtime.logger.Info("[model-auth] list failed", "error", device.Redact(err.Error()), "decision", "missing_or_pending")
			}
		} else {
			profiles = got
		}
	}
	status := modelauth.Classify(profiles, pending)
	if status == modelauth.OauthReady {
		runtime.mu.Lock()
		runtime.modelAuthPending = false
		runtime.mu.Unlock()
	}
	runtime.mu.Lock()
	changed := runtime.lastModelAuth != string(status)
	runtime.lastModelAuth = string(status)
	runtime.mu.Unlock()
	if changed {
		runtime.logger.Info("[model-auth] health", "status", string(status), "profile_count", len(profiles), "pending", pending)
	}
	return string(status)
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
	go runtime.preferOAuthAfterLogin()
	return prompt, nil
}

func (runtime *Runtime) preferOAuthAfterLogin() {
	if runtime == nil {
		return
	}
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
		ids := modelauth.PreferOrder(profiles)
		runtime.mu.Lock()
		runtime.modelAuthPending = false
		runtime.mu.Unlock()
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
		runtime.preferOAuthAfterLogin()
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

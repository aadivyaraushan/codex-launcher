package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/codex-launcher/codex-launcher/companion/internal/agent/taskstate"
	"github.com/codex-launcher/codex-launcher/companion/internal/codex/appserver"
	"github.com/codex-launcher/codex-launcher/companion/internal/decisions"
)

const decisionLifetime = 10 * time.Minute

type decisionTaskReader interface {
	CurrentTask(context.Context, string) (taskstate.Task, error)
}

type decisionPublisher interface {
	PublishTaskEvent(context.Context, taskstate.MobileEvent) error
}

func pumpDecisionRequests(
	ctx context.Context,
	requests <-chan appserver.ServerRequest,
	owner *decisions.AppServerOwner,
	router *decisions.Router,
	tasks decisionTaskReader,
	publisher decisionPublisher,
	computerName string,
	logger *slog.Logger,
	now func() time.Time,
	desktopOwned bool,
) {
	if logger == nil {
		logger = slog.Default()
	}
	if requests == nil || owner == nil || router == nil || tasks == nil || publisher == nil || now == nil {
		logger.Error("[decisions] request pump unavailable", "branch_reason", "missing_dependency")
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case request, open := <-requests:
			if !open {
				return
			}
			projectLabel := "Codex task"
			if task, err := tasks.CurrentTask(ctx, request.ThreadID); err == nil && task.ProjectLabel != "" {
				projectLabel = task.ProjectLabel
			} else if err != nil {
				logger.Warn("[decisions] task display context unavailable", "thread_id", request.ThreadID, "request_method", request.Method, "error_class", fmt.Sprintf("%T", err), "decision", "use_generic_project_label")
			}
			var projected decisions.Request
			var err error
			if desktopOwned {
				projected, err = owner.RegisterDesktop(request, decisions.DisplayContext{ComputerName: computerName, ProjectLabel: projectLabel}, now().Add(decisionLifetime))
			} else {
				projected, err = owner.Register(request, decisions.DisplayContext{ComputerName: computerName, ProjectLabel: projectLabel}, now().Add(decisionLifetime))
			}
			if err != nil {
				logger.Error("[decisions] request projection rejected", "thread_id", request.ThreadID, "request_method", request.Method, "error_class", fmt.Sprintf("%T", err), "decision", "keep_on_computer")
				continue
			}
			if err := router.Add(projected); err != nil {
				owner.Forget(projected.ID)
				logger.Error("[decisions] request registration failed", "request_id", projected.ID, "thread_id", request.ThreadID, "request_kind", projected.Kind, "error_class", fmt.Sprintf("%T", err), "decision", "keep_on_computer")
				continue
			}
			state := taskstate.WaitingForApproval
			kind := "approval"
			summary := "Codex needs your approval"
			if projected.Kind == decisions.KindQuestion {
				state = taskstate.WaitingForAnswer
				kind = "answer"
				summary = "Codex needs your answer"
			}
			if err := publisher.PublishTaskEvent(ctx, taskstate.MobileEvent{TaskID: request.ThreadID, Kind: kind, State: state, Summary: summary}); err != nil {
				logger.Error("[decisions] generic attention event failed", "request_id", projected.ID, "thread_id", request.ThreadID, "request_kind", projected.Kind, "error_class", fmt.Sprintf("%T", err), "decision", "retain_live_request")
				continue
			}
			logger.Info("[decisions] request ready for phone", "request_id", projected.ID, "thread_id", request.ThreadID, "request_kind", projected.Kind, "decision", "await_exact_response")
		}
	}
}

package claude

import (
	"context"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

type Pipeline struct {
	beforeMap map[string]func(pipeline.SendInput) pipeline.BeforeResult
	sendMap   map[string]pipeline.SendHandler
}

func New() *Pipeline {
	return &Pipeline{
		beforeMap: map[string]func(pipeline.SendInput) pipeline.BeforeResult{
			"/clear": pipeline.ClearCommandBefore,
			"/new":   pipeline.NewCommandBefore,
		},
		sendMap: map[string]pipeline.SendHandler{
			"resume": buildResumeCommand,
			"new":    buildNewCommand,
		},
	}
}

func (p *Pipeline) Name() string { return "claude" }

func (p *Pipeline) Mode() pipeline.Mode { return pipeline.ModeStream }

func (p *Pipeline) StderrMode() pipeline.StderrMode { return pipeline.StderrCapture }

func (p *Pipeline) EmitExitError() bool { return false }

func (p *Pipeline) NoContentFallback() string { return "" }

func (p *Pipeline) Send(ctx context.Context, input pipeline.SendInput) (pipeline.SendResult, error) {
	if err := pipeline.ValidateSendInput(input); err != nil {
		return pipeline.SendResult{}, err
	}
	if result, ok := pipeline.ResolveBefore(input, p.beforeMap); ok {
		return result, nil
	}

	key := "new"
	if input.AgentSessionID != "" {
		key = "resume"
	}
	return pipeline.BuildSendResult(ctx, input, key, p.sendMap)
}

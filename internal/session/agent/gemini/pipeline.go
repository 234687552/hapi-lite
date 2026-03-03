package gemini

import (
	"context"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

type Pipeline struct {
	beforeMap map[string]func(pipeline.SendInput) pipeline.BeforeResult
	send      pipeline.SendHandler
}

func New() *Pipeline {
	return &Pipeline{
		beforeMap: map[string]func(pipeline.SendInput) pipeline.BeforeResult{
			"/clear": pipeline.ClearCommandBefore,
			"/new":   pipeline.NewCommandBefore,
		},
		send: buildDefaultCommand,
	}
}

func (p *Pipeline) Name() string { return "gemini" }

func (p *Pipeline) Mode() pipeline.Mode { return pipeline.ModeDetached }

func (p *Pipeline) StderrMode() pipeline.StderrMode { return pipeline.StderrPassthrough }

func (p *Pipeline) EmitExitError() bool { return true }

func (p *Pipeline) NoContentFallback() string { return "" }

func (p *Pipeline) Send(ctx context.Context, input pipeline.SendInput) (pipeline.SendResult, error) {
	if err := pipeline.ValidateSendInput(input); err != nil {
		return pipeline.SendResult{}, err
	}
	if result, ok := pipeline.ResolveBefore(input, p.beforeMap); ok {
		return result, nil
	}
	return pipeline.BuildSingleSendResult(ctx, input, p.send)
}

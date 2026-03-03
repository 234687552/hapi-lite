package codex

import (
	"context"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

type afterEventHandler func(event map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action
type afterItemHandler func(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action

const (
	codexEventThreadStarted = "thread.started"
	codexEventItemCompleted = "item.completed"
)

type Pipeline struct {
	beforeMap    map[string]func(pipeline.SendInput) pipeline.BeforeResult
	sendMap      map[string]pipeline.SendHandler
	afterMap     map[string]afterEventHandler
	afterItemMap map[string]afterItemHandler
}

func New() *Pipeline {
	p := &Pipeline{
		beforeMap: map[string]func(pipeline.SendInput) pipeline.BeforeResult{
			"/clear": pipeline.ClearCommandBefore,
			"/new":   pipeline.NewCommandBefore,
		},
		sendMap: map[string]pipeline.SendHandler{
			"resume": buildResumeCommand,
			"new":    buildNewCommand,
		},
		afterItemMap: map[string]afterItemHandler{
			"agent_message":     handleAgentMessageItem,
			"reasoning":         handleReasoningItem,
			"tool_call":         handleToolCallItem,
			"web_search":        handleWebSearchItem,
			"command_execution": handleCommandExecutionItem,
			"tool_result":       handleToolResultItem,
		},
	}
	p.afterMap = map[string]afterEventHandler{
		codexEventThreadStarted: p.handleThreadStarted,
		codexEventItemCompleted: p.handleItemCompleted,
	}
	return p
}

func (p *Pipeline) Name() string { return "codex" }

func (p *Pipeline) Mode() pipeline.Mode { return pipeline.ModeStream }

func (p *Pipeline) StderrMode() pipeline.StderrMode { return pipeline.StderrPassthrough }

func (p *Pipeline) EmitExitError() bool { return true }

func (p *Pipeline) NoContentFallback() string { return "codex finished without response content" }

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

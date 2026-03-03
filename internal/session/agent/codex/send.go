package codex

import (
	"context"
	"os/exec"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

func buildResumeCommand(ctx context.Context, input pipeline.SendInput) (pipeline.CommandOutput, error) {
	args := []string{"exec", "resume", "--json", "--skip-git-repo-check"}
	if input.Request.Yolo {
		args = append(args, "--full-auto")
	}
	if input.Request.Model != "" && input.Request.Model != "default" {
		args = append(args, "--model", input.Request.Model)
	}
	args = append(args, input.AgentSessionID, input.Text)
	return pipeline.CommandOutput{
		Cmd:            exec.CommandContext(ctx, "codex", args...),
		AgentSessionID: input.AgentSessionID,
	}, nil
}

func buildNewCommand(ctx context.Context, input pipeline.SendInput) (pipeline.CommandOutput, error) {
	args := []string{"exec", "--json", "--skip-git-repo-check"}
	if input.Request.Yolo {
		args = append(args, "--full-auto")
	}
	if input.Request.Model != "" && input.Request.Model != "default" {
		args = append(args, "--model", input.Request.Model)
	}
	args = append(args, input.Text)
	return pipeline.CommandOutput{Cmd: exec.CommandContext(ctx, "codex", args...)}, nil
}

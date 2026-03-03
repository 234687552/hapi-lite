package opencode

import (
	"context"
	"os/exec"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

func buildDefaultCommand(ctx context.Context, input pipeline.SendInput) (pipeline.CommandOutput, error) {
	return pipeline.CommandOutput{
		Cmd: exec.CommandContext(ctx, "opencode", input.Text),
	}, nil
}

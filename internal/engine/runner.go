package engine

import (
	"context"
	"io"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type ExecRunner struct{}

func NewExecRunner() CommandRunner {
	return &ExecRunner{}
}

func (r *ExecRunner) Run(ctx context.Context, workDir string, command []string, output io.Writer) error {
	if len(command) == 0 {
		return nil
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(shellCommand(command)), "")
	if err != nil {
		return oops.In("bu1ld.engine").
			With("work_dir", workDir).
			With("command", command).
			Wrapf(err, "parse command")
	}
	runner, err := interp.New(interp.Dir(workDir), interp.StdIO(nil, output, output))
	if err != nil {
		return oops.In("bu1ld.engine").
			With("work_dir", workDir).
			With("command", command).
			Wrapf(err, "create command runner")
	}
	if err := runner.Run(ctx, file); err != nil {
		return oops.In("bu1ld.engine").
			With("work_dir", workDir).
			With("command", command).
			Wrapf(err, "run command %q", command[0])
	}
	return nil
}

func shellCommand(command []string) string {
	parts := list.MapList[string, string](list.NewList(command...), func(_ int, arg string) string {
		return shellQuote(arg)
	}).Values()
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

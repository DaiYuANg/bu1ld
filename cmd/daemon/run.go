package main

import (
	"bu1ld/internal/app"
	"bu1ld/internal/config"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func runCommand(cmd *cobra.Command, opts *options, request app.CommandRequest) error {
	cfg, err := config.New(opts.projectDir, opts.buildFile, opts.cacheDir, opts.noCache)
	if err != nil {
		return oops.In("bu1ld.daemon").
			With("command", request.Kind).
			With("project_dir", opts.projectDir).
			Wrapf(err, "load command configuration")
	}
	if err := app.RunCommand(cmd.Context(), cfg, opts.out, request); err != nil {
		return oops.In("bu1ld.daemon").
			With("command", request.Kind).
			With("project_dir", cfg.WorkDir).
			Wrapf(err, "run command")
	}
	return nil
}

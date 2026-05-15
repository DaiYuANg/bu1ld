package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"
	"github.com/lyonbrown4d/bu1ld/internal/config"
	builddaemon "github.com/lyonbrown4d/bu1ld/internal/daemon"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func loadConfig(opts *options) (config.Config, error) {
	return config.New(
		opts.projectDir,
		opts.buildFile,
		opts.cacheDir,
		opts.noCache,
		opts.remoteCacheURL,
		opts.remoteCachePull,
		opts.remoteCachePush,
	)
}

func runCommand(cmd *cobra.Command, opts *options, request app.CommandRequest) error {
	cfg, err := loadConfig(opts)
	if err != nil {
		return oops.In("bu1ld").
			With("command", request.Kind).
			With("project_dir", opts.projectDir).
			Wrapf(err, "load command configuration")
	}
	if !opts.noDaemon {
		ran, err := builddaemon.TryRun(cmd.Context(), cfg, opts.out, request)
		if ran || err != nil {
			if err != nil {
				return oops.In("bu1ld").
					With("command", request.Kind).
					With("project_dir", cfg.WorkDir).
					Wrapf(err, "run command through daemon")
			}
			return nil
		}
	}
	if err := app.RunCommand(cmd.Context(), cfg, opts.out, request); err != nil {
		return oops.In("bu1ld").
			With("command", request.Kind).
			With("project_dir", cfg.WorkDir).
			Wrapf(err, "run command")
	}
	return nil
}

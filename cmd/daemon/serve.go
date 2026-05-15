package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"
	builddaemon "github.com/lyonbrown4d/bu1ld/internal/daemon"

	"github.com/spf13/cobra"
)

func newRunCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:    "run",
		Short:  "Run the local build daemon in the foreground",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			return builddaemon.Serve(cmd.Context(), cfg, opts.out, app.RunCommand)
		},
	}
}

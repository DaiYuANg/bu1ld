package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newTestCommand(opts *options) *cobra.Command {
	var allPackages bool
	cmd := &cobra.Command{
		Use:   "test [task...]",
		Short: "Run test tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := args
			if len(targets) == 0 {
				targets = []string{"test"}
			}
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:        app.CommandTest,
				Targets:     targets,
				AllPackages: allPackages,
			})
		},
	}
	cmd.Flags().BoolVar(&allPackages, "all", false, "run local test targets in all workspace packages")
	return cmd
}

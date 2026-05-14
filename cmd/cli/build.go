package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newBuildCommand(opts *options) *cobra.Command {
	var allPackages bool
	cmd := &cobra.Command{
		Use:   "build [task...]",
		Short: "Build one or more tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			targets := args
			if len(targets) == 0 {
				targets = []string{"build"}
			}
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:        app.CommandBuild,
				Targets:     targets,
				AllPackages: allPackages,
			})
		},
	}
	cmd.Flags().BoolVar(&allPackages, "all", false, "run local task targets in all workspace packages")
	return cmd
}

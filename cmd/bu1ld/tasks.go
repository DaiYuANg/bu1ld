package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newTasksCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "tasks",
		Short: "List tasks declared in the build file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind: app.CommandTasks,
			})
		},
	}
}

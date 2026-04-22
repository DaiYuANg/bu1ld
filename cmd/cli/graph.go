package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newGraphCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph [task...]",
		Short: "Print the task graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:    app.CommandGraph,
				Targets: args,
			})
		},
	}
	return cmd
}

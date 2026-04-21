package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newCoordinatorCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "coordinator",
		Short: "Start a distributed build coordinator",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandServerCoordinator})
		},
	}
}

package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newCoordinatorCommand(opts *options) *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "coordinator",
		Short: "Start a distributed build coordinator",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:       app.CommandServerCoordinator,
				ListenAddr: listen,
			})
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "coordinator listen address")
	return cmd
}

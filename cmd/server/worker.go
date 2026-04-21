package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newWorkerCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Start a distributed build worker",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandServerWorker})
		},
	}
}

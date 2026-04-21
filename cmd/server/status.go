package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show distributed build service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandServerStatus})
		},
	}
}

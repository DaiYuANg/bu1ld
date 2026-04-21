package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newCleanCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean local build cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind: app.CommandClean,
			})
		},
	}
	return cmd
}

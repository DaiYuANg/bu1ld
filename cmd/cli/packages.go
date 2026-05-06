package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newPackagesCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "packages",
		Short: "Inspect workspace packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandPackages})
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "graph",
		Short: "Print the workspace package dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandPackagesGraph})
		},
	})
	return cmd
}

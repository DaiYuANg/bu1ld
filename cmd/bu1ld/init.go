package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newInitCommand(opts *options) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter bu1ld project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:       app.CommandInit,
				ForceWrite: force,
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing starter files")
	return cmd
}

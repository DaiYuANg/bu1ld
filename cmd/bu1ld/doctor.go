package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newDoctorCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check project configuration, graph, and plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind: app.CommandDoctor,
			})
		},
	}
}

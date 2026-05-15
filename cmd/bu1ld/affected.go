package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newAffectedCommand(opts *options) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "affected",
		Short: "List workspace packages affected by git changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:    app.CommandAffected,
				BaseRef: base,
			})
		},
	}
	cmd.Flags().StringVar(&base, "base", "HEAD", "base git revision for changed-file detection")
	return cmd
}

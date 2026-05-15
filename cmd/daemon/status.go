package main

import (
	"fmt"
	builddaemon "github.com/lyonbrown4d/bu1ld/internal/daemon"

	"github.com/spf13/cobra"
)

func newStatusCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			status, err := builddaemon.Check(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(opts.out, builddaemon.FormatStatus(status))
			return err
		},
	}
}

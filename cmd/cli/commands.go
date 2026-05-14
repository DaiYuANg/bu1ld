package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

type childCommandSpec struct {
	use     string
	short   string
	request app.CommandKind
}

func newCommandGroup(use, short string, opts *options, children ...childCommandSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, child := range children {
		cmd.AddCommand(newChildCommand(opts, child))
	}
	return cmd
}

func newChildCommand(opts *options, spec childCommandSpec) *cobra.Command {
	return &cobra.Command{
		Use:   spec.use,
		Short: spec.short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: spec.request})
		},
	}
}

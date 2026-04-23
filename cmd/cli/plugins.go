package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newPluginsCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Inspect build plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List builtin and declared plugins",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandPluginsList})
			},
		},
		&cobra.Command{
			Use:   "doctor",
			Short: "Check declared plugin health",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandPluginsDoctor})
			},
		},
		&cobra.Command{
			Use:   "lock",
			Short: "Write the plugin lockfile",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandPluginsLock})
			},
		},
	)
	return cmd
}

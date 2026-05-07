package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newPluginsCommand(opts *options) *cobra.Command {
	cmd := newCommandGroup("plugins", "Inspect build plugins", opts,
		childCommandSpec{use: "list", short: "List builtin and declared plugins", request: app.CommandPluginsList},
		childCommandSpec{use: "doctor", short: "Check declared plugin health", request: app.CommandPluginsDoctor},
		childCommandSpec{use: "lock", short: "Write the plugin lockfile", request: app.CommandPluginsLock},
	)
	cmd.AddCommand(
		newPluginsSearchCommand(opts),
		newPluginsInfoCommand(opts),
		newPluginsInstallCommand(opts),
		newPluginsUpdateCommand(opts),
	)
	return cmd
}

func newPluginsSearchCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Search the plugin registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:        app.CommandPluginsSearch,
				PluginQuery: query,
			})
		},
	}
}

func newPluginsInfoCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <plugin[@version]>",
		Short: "Show plugin registry details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:      app.CommandPluginsInfo,
				PluginRef: args[0],
			})
		},
	}
}

func newPluginsInstallCommand(opts *options) *cobra.Command {
	var global bool
	var force bool
	cmd := &cobra.Command{
		Use:   "install <plugin[@version]>",
		Short: "Install a plugin from the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:         app.CommandPluginsInstall,
				PluginRef:    args[0],
				PluginGlobal: global,
				ForceWrite:   force,
			})
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "install into the global plugin directory")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing installed plugin version")
	return cmd
}

func newPluginsUpdateCommand(opts *options) *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "update <plugin[@version]>",
		Short: "Update a plugin from the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:         app.CommandPluginsUpdate,
				PluginRef:    args[0],
				PluginGlobal: global,
				ForceWrite:   true,
			})
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "update the global plugin directory")
	return cmd
}

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
		newPluginsRegistryCommand(opts),
		newPluginsPublishCommand(opts),
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

func newPluginsRegistryCommand(opts *options) *cobra.Command {
	cmd := newCommandGroup("registry", "Manage plugin registry metadata", opts)
	cmd.AddCommand(&cobra.Command{
		Use:   "validate [source]",
		Short: "Validate plugin registry metadata",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := ""
			if len(args) > 0 {
				source = args[0]
			}
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:           app.CommandPluginsRegistryValidate,
				RegistrySource: source,
			})
		},
	})
	return cmd
}

func newPluginsPublishCommand(opts *options) *cobra.Command {
	var assetURL string
	var goos string
	var goarch string
	var format string
	var sha256 string
	var status string
	var bu1ld string
	cmd := &cobra.Command{
		Use:   "publish <plugin.toml>",
		Short: "Generate registry metadata for a plugin artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{
				Kind:               app.CommandPluginsPublish,
				PluginManifestPath: args[0],
				PluginAssetURL:     assetURL,
				PluginOS:           goos,
				PluginArch:         goarch,
				PluginFormat:       format,
				PluginSHA256:       sha256,
				PluginStatus:       status,
				PluginBu1ld:        bu1ld,
			})
		},
	}
	cmd.Flags().StringVar(&assetURL, "asset-url", "", "download URL for the plugin artifact")
	cmd.Flags().StringVar(&goos, "os", "", "target operating system for the asset")
	cmd.Flags().StringVar(&goarch, "arch", "", "target architecture for the asset")
	cmd.Flags().StringVar(&format, "format", "", "asset format: zip, tar, tar.gz, tgz, or dir")
	cmd.Flags().StringVar(&sha256, "sha256", "", "asset SHA-256 checksum")
	cmd.Flags().StringVar(&status, "status", "approved", "registry version status")
	cmd.Flags().StringVar(&bu1ld, "bu1ld", "", "compatible bu1ld version constraint")
	return cmd
}

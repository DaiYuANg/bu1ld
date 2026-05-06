package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newPluginsCommand(opts *options) *cobra.Command {
	return newCommandGroup("plugins", "Inspect build plugins", opts,
		childCommandSpec{use: "list", short: "List builtin and declared plugins", request: app.CommandPluginsList},
		childCommandSpec{use: "doctor", short: "Check declared plugin health", request: app.CommandPluginsDoctor},
		childCommandSpec{use: "lock", short: "Write the plugin lockfile", request: app.CommandPluginsLock},
	)
}

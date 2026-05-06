package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newDaemonCommand(opts *options) *cobra.Command {
	return newCommandGroup("daemon", "Manage the local build daemon", opts,
		childCommandSpec{use: "status", short: "Show local daemon status", request: app.CommandDaemonStatus},
		childCommandSpec{use: "start", short: "Start the local build daemon", request: app.CommandDaemonStart},
		childCommandSpec{use: "stop", short: "Stop the local build daemon", request: app.CommandDaemonStop},
	)
}

package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newServerCommand(opts *options) *cobra.Command {
	return newCommandGroup("server", "Manage distributed build services", opts,
		childCommandSpec{use: "status", short: "Show distributed build service status", request: app.CommandServerStatus},
		childCommandSpec{use: "coordinator", short: "Start a distributed build coordinator", request: app.CommandServerCoordinator},
		childCommandSpec{use: "worker", short: "Start a distributed build worker", request: app.CommandServerWorker},
	)
}

package main

import (
	"fmt"

	"github.com/lyonbrown4d/bu1ld/internal/app"
	builddaemon "github.com/lyonbrown4d/bu1ld/internal/daemon"

	"github.com/spf13/cobra"
)

func newDaemonCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newDaemonStatusCommand(opts),
		newDaemonStartCommand(opts),
		newDaemonStopCommand(opts),
		newDaemonRunCommand(opts),
	)
	return cmd
}

func newDaemonStatusCommand(opts *options) *cobra.Command {
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

func newDaemonStartCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			state, alreadyRunning, err := builddaemon.Start(cmd.Context(), cfg, []string{"daemon", "run"})
			if err != nil {
				return err
			}
			if alreadyRunning {
				_, err = fmt.Fprintf(opts.out, "daemon already running on %s\n", state.Endpoint)
				return err
			}
			_, err = fmt.Fprintf(opts.out, "daemon started on %s\n", state.Endpoint)
			return err
		},
	}
}

func newDaemonStopCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			state, stopped, err := builddaemon.Stop(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			if !stopped {
				_, err = fmt.Fprintln(opts.out, "daemon status: stopped")
				return err
			}
			_, err = fmt.Fprintf(opts.out, "daemon stopped on %s\n", state.Endpoint)
			return err
		},
	}
}

func newDaemonRunCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:    "run",
		Short:  "Run the local build daemon in the foreground",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			return builddaemon.Serve(cmd.Context(), cfg, opts.out, app.RunCommand)
		},
	}
}

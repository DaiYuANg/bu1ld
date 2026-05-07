package main

import (
	"io"
	"os"

	"bu1ld/internal/clierr"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

type options struct {
	projectDir      string
	buildFile       string
	cacheDir        string
	noCache         bool
	remoteCacheURL  string
	remoteCachePull bool
	remoteCachePush bool
	out             io.Writer
}

func Execute() error {
	cmd := NewRootCommand(os.Stdout)
	cmd.SetErr(os.Stderr)

	if err := cmd.Execute(); err != nil {
		if printErr := clierr.Print(cmd.ErrOrStderr(), err); printErr != nil {
			return oops.In("bu1ld.server").Wrapf(printErr, "print server error")
		}
		return oops.In("bu1ld.server").Wrapf(err, "execute server command")
	}
	return nil
}

func NewRootCommand(out io.Writer) *cobra.Command {
	opts := options{
		projectDir:      ".",
		buildFile:       "build.bu1ld",
		cacheDir:        ".bu1ld/cache",
		remoteCachePull: true,
		out:             out,
	}

	cmd := &cobra.Command{
		Use:           "server",
		Short:         "Distributed bu1ld server process",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetOut(out)
	cmd.SetErr(out)

	cmd.PersistentFlags().StringVar(&opts.projectDir, "project-dir", opts.projectDir, "project directory")
	cmd.PersistentFlags().StringVarP(&opts.buildFile, "file", "f", opts.buildFile, "build DSL file")
	cmd.PersistentFlags().StringVar(&opts.cacheDir, "cache-dir", opts.cacheDir, "build cache directory")
	cmd.PersistentFlags().BoolVar(&opts.noCache, "no-cache", false, "disable build cache reads and writes")
	cmd.PersistentFlags().StringVar(&opts.remoteCacheURL, "remote-cache-url", opts.remoteCacheURL, "remote build cache base URL")
	cmd.PersistentFlags().BoolVar(&opts.remoteCachePull, "remote-cache-pull", opts.remoteCachePull, "pull from remote build cache when configured")
	cmd.PersistentFlags().BoolVar(&opts.remoteCachePush, "remote-cache-push", opts.remoteCachePush, "push local build outputs to remote build cache when configured")

	cmd.AddCommand(
		newStatusCommand(&opts),
		newCoordinatorCommand(&opts),
		newWorkerCommand(&opts),
	)
	return cmd
}

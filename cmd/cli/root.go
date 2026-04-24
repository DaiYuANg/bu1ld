package main

import (
	"io"
	"os"

	"bu1ld/internal/clierr"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

type options struct {
	projectDir string
	buildFile  string
	cacheDir   string
	noCache    bool
	out        io.Writer
}

func Execute() error {
	cmd := NewRootCommand(os.Stdout)
	cmd.SetErr(os.Stderr)

	if err := cmd.Execute(); err != nil {
		if printErr := clierr.Print(cmd.ErrOrStderr(), err); printErr != nil {
			return oops.In("bu1ld.cli").Wrapf(printErr, "print cli error")
		}
		return oops.In("bu1ld.cli").Wrapf(err, "execute cli command")
	}

	return nil
}

func NewRootCommand(out io.Writer) *cobra.Command {
	opts := options{
		projectDir: ".",
		buildFile:  "build.bu1ld",
		cacheDir:   ".bu1ld/cache",
		out:        out,
	}

	cmd := &cobra.Command{
		Use:           "bu1ld",
		Short:         "A cross-language build tool",
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

	cmd.AddCommand(
		newBuildCommand(&opts),
		newTestCommand(&opts),
		newTasksCommand(&opts),
		newGraphCommand(&opts),
		newCleanCommand(&opts),
		newPluginsCommand(&opts),
		newDaemonCommand(&opts),
		newServerCommand(&opts),
	)

	return cmd
}

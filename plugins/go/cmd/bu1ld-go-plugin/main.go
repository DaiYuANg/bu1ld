package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"bu1ld/internal/gocacheprog"
	"bu1ld/pkg/pluginapi"
	goplugin "bu1ld/plugins/go"
	"github.com/arcgolabs/dix"
	"github.com/spf13/cobra"
)

func main() {
	if err := run(context.Background(), commandStreams{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type commandStreams struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func run(ctx context.Context, streams commandStreams) error {
	return newRootCommand(ctx, streams).ExecuteContext(ctx)
}

func newRootCommand(ctx context.Context, streams commandStreams) *cobra.Command {
	command := &cobra.Command{
		Use:           "bu1ld-go-plugin",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return runPlugin(ctx)
		},
	}
	command.SetIn(streams.input())
	command.SetOut(streams.output())
	command.SetErr(streams.errorOutput())
	command.AddCommand(newCacheprogCommand(ctx, streams))
	return command
}

func newCacheprogCommand(ctx context.Context, streams commandStreams) *cobra.Command {
	options := gocacheprog.OptionsFromEnv()
	command := &cobra.Command{
		Use:           "cacheprog",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return gocacheprog.Serve(ctx, streams.input(), streams.output(), options)
		},
	}
	flags := command.Flags()
	flags.StringVar(&options.RemoteCacheURL, "remote-cache-url", options.RemoteCacheURL, "bu1ld coordinator remote cache URL")
	flags.StringVar(&options.RemoteCacheURL, "url", options.RemoteCacheURL, "bu1ld coordinator remote cache URL")
	flags.StringVar(&options.RemoteCacheToken, "remote-cache-token", options.RemoteCacheToken, "bearer token for the bu1ld coordinator remote cache")
	flags.StringVar(&options.CacheDir, "cache-dir", options.CacheDir, "local GOCACHEPROG disk path")
	flags.BoolVar(&options.RemotePull, "remote-cache-pull", options.RemotePull, "pull Go cache entries from the coordinator")
	flags.BoolVar(&options.RemotePush, "remote-cache-push", options.RemotePush, "push Go cache entries to the coordinator")
	flags.StringVar(&options.LogPath, "log", options.LogPath, "write cacheprog hit/miss diagnostics to a file")
	return command
}

func (s commandStreams) input() io.Reader {
	if s.stdin == nil {
		return bytes.NewReader(nil)
	}
	return s.stdin
}

func (s commandStreams) output() io.Writer {
	if s.stdout == nil {
		return io.Discard
	}
	return s.stdout
}

func (s commandStreams) errorOutput() io.Writer {
	if s.stderr == nil {
		return io.Discard
	}
	return s.stderr
}

func runPlugin(ctx context.Context) (err error) {
	spec := dix.New(
		"bu1ld go plugin",
		dix.Modules(goPluginModule()),
		dix.UseLogger1(func(logger *slog.Logger) *slog.Logger {
			return logger
		}),
	)
	runtime, err := spec.Start(ctx)
	if err != nil {
		return fmt.Errorf("start plugin runtime: %w", err)
	}
	defer func() {
		if stopErr := runtime.Stop(context.WithoutCancel(ctx)); stopErr != nil {
			err = errors.Join(err, fmt.Errorf("stop plugin runtime: %w", stopErr))
		}
	}()

	item, err := dix.ResolveAs[pluginapi.Plugin](runtime.Container())
	if err != nil {
		return fmt.Errorf("resolve plugin: %w", err)
	}
	if err := pluginapi.ServeProcess(item); err != nil {
		return fmt.Errorf("serve plugin process: %w", err)
	}
	return nil
}

func goPluginModule() dix.Module {
	return dix.NewModule("go-build-plugin",
		dix.Providers(
			dix.Value[*slog.Logger](slog.New(slog.DiscardHandler)),
			dix.Provider0[*goplugin.Plugin](goplugin.New),
			dix.Provider1[pluginapi.Plugin, *goplugin.Plugin](func(plugin *goplugin.Plugin) pluginapi.Plugin {
				return plugin
			}),
		),
	)
}

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"bu1ld/pkg/pluginapi"
	goplugin "bu1ld/plugins/go"
	"github.com/arcgolabs/dix"
)

func main() {
	if err := run(context.Background()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (err error) {
	spec := dix.New(
		"bu1ld go plugin",
		dix.Modules(goPluginModule()),
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
		dix.WithModuleProviders(
			dix.Provider0[*goplugin.Plugin](goplugin.New),
			dix.Provider1[pluginapi.Plugin, *goplugin.Plugin](func(plugin *goplugin.Plugin) pluginapi.Plugin {
				return plugin
			}),
		),
	)
}

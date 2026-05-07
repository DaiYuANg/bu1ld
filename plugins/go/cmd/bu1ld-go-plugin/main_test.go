package main

import (
	"context"
	"testing"

	"bu1ld/pkg/pluginapi"
	"github.com/arcgolabs/dix"
)

func TestGoPluginModuleResolvesPlugin(t *testing.T) {
	t.Parallel()

	spec := dix.New("test go plugin", dix.Modules(goPluginModule()))
	runtime, err := spec.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	item, err := dix.ResolveAs[pluginapi.Plugin](runtime.Container())
	if err != nil {
		t.Fatalf("ResolveAs() error = %v", err)
	}
	metadata, err := item.Metadata()
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if got, want := metadata.ID, "org.bu1ld.go"; got != want {
		t.Fatalf("metadata id = %q, want %q", got, want)
	}
}

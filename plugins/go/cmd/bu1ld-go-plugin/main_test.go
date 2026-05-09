package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"bu1ld/internal/gocacheprog"
	"bu1ld/pkg/pluginapi"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/dix/testx"
)

func TestGoPluginModuleResolvesPlugin(t *testing.T) {
	t.Parallel()

	spec := dix.New("test go plugin", dix.Modules(goPluginModule()))
	runtime := testx.Start(context.Background(), t, spec)

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
	if got, want := metadata.ProtocolVersion, pluginapi.ProtocolVersion; got != want {
		t.Fatalf("metadata protocol version = %d, want %d", got, want)
	}
	if _, ok := item.(pluginapi.ExecutablePlugin); !ok {
		t.Fatalf("resolved plugin does not implement ExecutablePlugin")
	}
}

func TestCacheprogCommandParsesCobraFlagsAndServes(t *testing.T) {
	t.Parallel()

	var input bytes.Buffer
	if err := json.NewEncoder(&input).Encode(gocacheprog.Request{
		ID:      1,
		Command: gocacheprog.CmdClose,
	}); err != nil {
		t.Fatalf("Encode(close) error = %v", err)
	}

	var output bytes.Buffer
	var stderr bytes.Buffer
	command := newRootCommand(context.Background(), commandStreams{
		stdin:  &input,
		stdout: &output,
		stderr: &stderr,
	})
	command.SetArgs([]string{
		"cacheprog",
		"--cache-dir", t.TempDir(),
		"--remote-cache-pull=false",
		"--remote-cache-push=true",
	})

	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	decoder := json.NewDecoder(&output)
	var capabilities gocacheprog.Response
	if err := decoder.Decode(&capabilities); err != nil {
		t.Fatalf("Decode(capabilities) error = %v", err)
	}
	if got, want := capabilities.ID, int64(0); got != want {
		t.Fatalf("capabilities ID = %d, want %d", got, want)
	}

	var close gocacheprog.Response
	if err := decoder.Decode(&close); err != nil {
		t.Fatalf("Decode(close) error = %v", err)
	}
	if got, want := close.ID, int64(1); got != want {
		t.Fatalf("close ID = %d, want %d", got, want)
	}
	if close.Err != "" {
		t.Fatalf("close Err = %q", close.Err)
	}
}

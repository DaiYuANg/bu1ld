package pluginapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestServeHandlesMetadataAndExpand(t *testing.T) {
	t.Parallel()

	var input bytes.Buffer
	writeTestMessage(t, &input, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "metadata"})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "configure",
		"params": map[string]any{
			"config": map[string]any{"namespace": "fake", "fields": map[string]any{"message": "configured"}},
		},
	})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "expand",
		"params": map[string]any{
			"invocation": map[string]any{
				"namespace": "fake",
				"rule":      "echo",
				"target":    "hello",
				"fields":    map[string]any{"message": "world"},
			},
		},
	})
	writeTestMessage(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "execute",
		"params": map[string]any{
			"request": map[string]any{
				"namespace": "fake",
				"action":    "echo",
				"work_dir":  ".",
				"params":    map[string]any{"message": "executed"},
			},
		},
	})
	var output bytes.Buffer

	if err := Serve(fakePlugin{}, &input, &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	text := output.String()
	for _, want := range []string{
		`"id":"org.bu1ld.fake"`,
		`"name":"configured"`,
		`"command":["echo","world"]`,
		`"output":"executed\n"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %s, want substring %q", text, want)
		}
	}
}

func writeTestMessage(t *testing.T, out *bytes.Buffer, message any) {
	t.Helper()

	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(payload), payload); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

type fakePlugin struct{}

func (p fakePlugin) Metadata() (Metadata, error) {
	return Metadata{
		ID:        "org.bu1ld.fake",
		Namespace: "fake",
		Rules: []RuleSchema{
			{
				Name: "echo",
				Fields: []FieldSchema{
					{Name: "message", Type: FieldString, Required: true},
				},
			},
		},
	}, nil
}

func (p fakePlugin) Expand(_ context.Context, invocation Invocation) ([]TaskSpec, error) {
	message, err := invocation.RequiredString("message")
	if err != nil {
		return nil, err
	}
	return []TaskSpec{{Name: invocation.Target, Command: []string{"echo", message}}}, nil
}

func (p fakePlugin) Configure(_ context.Context, config PluginConfig) ([]TaskSpec, error) {
	message, ok := config.Fields["message"].(string)
	if !ok {
		message = "configured"
	}
	return []TaskSpec{{Name: message}}, nil
}

func (p fakePlugin) Execute(_ context.Context, request ExecuteRequest) (ExecuteResult, error) {
	message, ok := request.Params["message"].(string)
	if !ok {
		message = "executed"
	}
	return ExecuteResult{Output: message + "\n"}, nil
}

package pluginapi

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServeHandlesMetadataAndExpand(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(strings.Join([]string{
		`{"id":1,"method":"metadata"}`,
		`{"id":2,"method":"configure","params":{"config":{"namespace":"fake","fields":{"message":"configured"}}}}`,
		`{"id":3,"method":"expand","params":{"invocation":{"namespace":"fake","rule":"echo","target":"hello","fields":{"message":"world"}}}}`,
		`{"id":4,"method":"execute","params":{"request":{"namespace":"fake","action":"echo","work_dir":".","params":{"message":"executed"}}}}`,
		"",
	}, "\n"))
	var output bytes.Buffer

	if err := Serve(fakePlugin{}, input, &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	decoder := json.NewDecoder(&output)
	var metadataResponse Response
	if err := decoder.Decode(&metadataResponse); err != nil {
		t.Fatalf("decode metadata response: %v", err)
	}
	var metadata MetadataResult
	if err := json.Unmarshal(metadataResponse.Result, &metadata); err != nil {
		t.Fatalf("decode metadata result: %v", err)
	}
	if got, want := metadata.Metadata.ID, "org.bu1ld.fake"; got != want {
		t.Fatalf("metadata id = %q, want %q", got, want)
	}

	var configureResponse Response
	if err := decoder.Decode(&configureResponse); err != nil {
		t.Fatalf("decode configure response: %v", err)
	}
	var configure ConfigureResult
	if err := json.Unmarshal(configureResponse.Result, &configure); err != nil {
		t.Fatalf("decode configure result: %v", err)
	}
	if got, want := configure.Tasks[0].Name, "configured"; got != want {
		t.Fatalf("configured task = %q, want %q", got, want)
	}

	var expandResponse Response
	if err := decoder.Decode(&expandResponse); err != nil {
		t.Fatalf("decode expand response: %v", err)
	}
	var expand ExpandResult
	if err := json.Unmarshal(expandResponse.Result, &expand); err != nil {
		t.Fatalf("decode expand result: %v", err)
	}
	if got, want := expand.Tasks[0].Command[1], "world"; got != want {
		t.Fatalf("command arg = %q, want %q", got, want)
	}

	var executeResponse Response
	if err := decoder.Decode(&executeResponse); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	var execute ExecuteResult
	if err := json.Unmarshal(executeResponse.Result, &execute); err != nil {
		t.Fatalf("decode execute result: %v", err)
	}
	if got, want := execute.Output, "executed\n"; got != want {
		t.Fatalf("execute output = %q, want %q", got, want)
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

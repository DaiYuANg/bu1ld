package pluginapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	MethodMetadata       = "metadata"
	MethodConfigure      = "configure"
	MethodExpand         = "expand"
	MethodExecute        = "execute"
	PluginExecActionKind = "plugin.exec"
)

type Request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Message string `json:"message"`
}

type MetadataResult struct {
	Metadata Metadata `json:"metadata"`
}

type ExpandParams struct {
	Invocation Invocation `json:"invocation"`
}

type ExpandResult struct {
	Tasks []TaskSpec `json:"tasks"`
}

type ConfigureParams struct {
	Config PluginConfig `json:"config"`
}

type ConfigureResult struct {
	Tasks []TaskSpec `json:"tasks"`
}

type ExecuteParams struct {
	Request ExecuteRequest `json:"request"`
}

func ServeProcess(item Plugin) error {
	return Serve(item, os.Stdin, os.Stdout)
}

func Serve(item Plugin, input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	encoder := json.NewEncoder(output)
	for {
		var request Request
		if err := decoder.Decode(&request); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode plugin request: %w", err)
		}
		response := handleRequest(item, request)
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("encode plugin response: %w", err)
		}
	}
}

func handleRequest(item Plugin, request Request) Response {
	switch request.Method {
	case MethodMetadata:
		metadata, err := item.Metadata()
		if err != nil {
			return errorResponse(request.ID, fmt.Errorf("read plugin metadata: %w", err))
		}
		return resultResponse(request.ID, MetadataResult{Metadata: metadata})
	case MethodConfigure:
		configurable, ok := item.(ConfigurablePlugin)
		if !ok {
			return errorResponse(request.ID, fmt.Errorf("plugin does not support configure"))
		}
		var params ConfigureParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return errorResponse(request.ID, fmt.Errorf("decode configure params: %w", err))
		}
		tasks, err := configurable.Configure(context.Background(), params.Config)
		if err != nil {
			return errorResponse(request.ID, fmt.Errorf("configure plugin: %w", err))
		}
		return resultResponse(request.ID, ConfigureResult{Tasks: tasks})
	case MethodExpand:
		var params ExpandParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return errorResponse(request.ID, fmt.Errorf("decode expand params: %w", err))
		}
		tasks, err := item.Expand(context.Background(), params.Invocation)
		if err != nil {
			return errorResponse(request.ID, fmt.Errorf("expand plugin invocation: %w", err))
		}
		return resultResponse(request.ID, ExpandResult{Tasks: tasks})
	case MethodExecute:
		executable, ok := item.(ExecutablePlugin)
		if !ok {
			return errorResponse(request.ID, fmt.Errorf("plugin does not support execute"))
		}
		var params ExecuteParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return errorResponse(request.ID, fmt.Errorf("decode execute params: %w", err))
		}
		result, err := executable.Execute(context.Background(), params.Request)
		if err != nil {
			return errorResponse(request.ID, fmt.Errorf("execute plugin action: %w", err))
		}
		return resultResponse(request.ID, result)
	default:
		return errorResponse(request.ID, fmt.Errorf("unknown plugin method %q", request.Method))
	}
}

func resultResponse(id int64, result any) Response {
	data, err := json.Marshal(result)
	if err != nil {
		return errorResponse(id, fmt.Errorf("marshal plugin result: %w", err))
	}
	return Response{ID: id, Result: data}
}

func errorResponse(id int64, err error) Response {
	return Response{ID: id, Error: &ResponseError{Message: err.Error()}}
}

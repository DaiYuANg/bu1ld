package pluginapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.lsp.dev/jsonrpc2"
)

const (
	MethodMetadata       = "metadata"
	MethodConfigure      = "configure"
	MethodExpand         = "expand"
	MethodExecute        = "execute"
	PluginExecActionKind = "plugin.exec"
)

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
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(readWriteCloser{
		Reader: input,
		Writer: output,
	}))
	conn.Go(context.Background(), func(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
		return handleRequest(ctx, item, reply, request)
	})
	<-conn.Done()
	if err := conn.Err(); err != nil && !isStreamEOF(err) {
		return fmt.Errorf("serve plugin JSON-RPC connection: %w", err)
	}
	return nil
}

func handleRequest(ctx context.Context, item Plugin, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	switch request.Method() {
	case MethodMetadata:
		metadata, err := item.Metadata()
		if err != nil {
			return reply(ctx, nil, fmt.Errorf("read plugin metadata: %w", err))
		}
		metadata = NormalizeMetadata(item, metadata)
		return reply(ctx, MetadataResult{Metadata: metadata}, nil)
	case MethodConfigure:
		configurable, ok := item.(ConfigurablePlugin)
		if !ok {
			return reply(ctx, nil, fmt.Errorf("plugin does not support configure"))
		}
		var params ConfigureParams
		if err := decodeParams(request, &params); err != nil {
			return reply(ctx, nil, err)
		}
		tasks, err := configurable.Configure(ctx, params.Config)
		if err != nil {
			return reply(ctx, nil, fmt.Errorf("configure plugin: %w", err))
		}
		return reply(ctx, ConfigureResult{Tasks: tasks}, nil)
	case MethodExpand:
		var params ExpandParams
		if err := decodeParams(request, &params); err != nil {
			return reply(ctx, nil, err)
		}
		tasks, err := item.Expand(ctx, params.Invocation)
		if err != nil {
			return reply(ctx, nil, fmt.Errorf("expand plugin invocation: %w", err))
		}
		return reply(ctx, ExpandResult{Tasks: tasks}, nil)
	case MethodExecute:
		executable, ok := item.(ExecutablePlugin)
		if !ok {
			return reply(ctx, nil, fmt.Errorf("plugin does not support execute"))
		}
		var params ExecuteParams
		if err := decodeParams(request, &params); err != nil {
			return reply(ctx, nil, err)
		}
		result, err := executable.Execute(ctx, params.Request)
		if err != nil {
			return reply(ctx, nil, fmt.Errorf("execute plugin action: %w", err))
		}
		return reply(ctx, result, nil)
	default:
		return reply(ctx, nil, fmt.Errorf("unknown plugin method %q", request.Method()))
	}
}

func decodeParams(request jsonrpc2.Request, target any) error {
	if err := json.Unmarshal(request.Params(), target); err != nil {
		return fmt.Errorf("decode %s params: %w", request.Method(), err)
	}
	return nil
}

type readWriteCloser struct {
	io.Reader
	io.Writer
}

func (c readWriteCloser) Close() error {
	return nil
}

func isStreamEOF(err error) bool {
	return errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF")
}

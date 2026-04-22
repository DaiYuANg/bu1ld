package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"bu1ld/internal/dsl"

	"github.com/DaiYuANg/arcgo/dix"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

var dslPositionPattern = regexp.MustCompile(`dsl:(\d+):(\d+):`)

type Server struct {
	parser *dsl.Parser
	in     io.Reader
	out    io.Writer
	conn   jsonrpc2.Conn
	docs   map[string]string
}

func New(parser *dsl.Parser, in io.Reader, out io.Writer) *Server {
	return &Server{
		parser: parser,
		in:     in,
		out:    out,
		docs:   map[string]string{},
	}
}

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	spec := NewDixApp(in, out)
	runtime, err := spec.Start(ctx)
	if err != nil {
		return fmt.Errorf("start lsp dix runtime: %w", err)
	}
	defer func() {
		stopCtx := context.WithoutCancel(ctx)
		_ = runtime.Stop(stopCtx)
	}()

	server, err := dix.ResolveAs[*Server](runtime.Container())
	if err != nil {
		return fmt.Errorf("resolve lsp server: %w", err)
	}
	return server.Serve(ctx)
}

func NewDixApp(in io.Reader, out io.Writer) *dix.App {
	return dix.New(
		"bu1ld lsp",
		dix.Modules(
			dix.NewModule("lsp",
				dix.WithModuleProviders(
					dix.Value[io.Reader](in),
					dix.Value[io.Writer](out),
					dix.Provider0(dsl.NewParser),
					dix.Provider3(New),
				),
			),
		),
	)
}

func (s *Server) Serve(ctx context.Context) error {
	s.conn = jsonrpc2.NewConn(jsonrpc2.NewStream(readWriteCloser{
		Reader: s.in,
		Writer: s.out,
	}))
	s.conn.Go(ctx, s.handle)
	<-s.conn.Done()
	err := s.conn.Err()
	if err == nil || isStreamEOF(err) {
		return nil
	}
	return err
}

func (s *Server) handle(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	switch request.Method() {
	case "initialize":
		return reply(ctx, protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				TextDocumentSync: protocol.TextDocumentSyncKindFull,
				CompletionProvider: &protocol.CompletionOptions{
					TriggerCharacters: []string{".", " ", "="},
				},
			},
		}, nil)
	case "initialized":
		return reply(ctx, nil, nil)
	case "shutdown":
		return reply(ctx, nil, nil)
	case "exit":
		_ = reply(ctx, nil, nil)
		return io.EOF
	case "textDocument/didOpen":
		var params protocol.DidOpenTextDocumentParams
		if err := decodeParams(request, &params); err != nil {
			return err
		}
		uri := string(params.TextDocument.URI)
		s.docs[uri] = params.TextDocument.Text
		if err := s.publishDiagnostics(ctx, params.TextDocument.URI, params.TextDocument.Text); err != nil {
			return err
		}
		return reply(ctx, nil, nil)
	case "textDocument/didChange":
		var params protocol.DidChangeTextDocumentParams
		if err := decodeParams(request, &params); err != nil {
			return err
		}
		uri := string(params.TextDocument.URI)
		text := s.docs[uri]
		if len(params.ContentChanges) > 0 {
			text = params.ContentChanges[len(params.ContentChanges)-1].Text
		}
		s.docs[uri] = text
		if err := s.publishDiagnostics(ctx, params.TextDocument.URI, text); err != nil {
			return err
		}
		return reply(ctx, nil, nil)
	case "textDocument/didClose":
		var params protocol.DidCloseTextDocumentParams
		if err := decodeParams(request, &params); err != nil {
			return err
		}
		delete(s.docs, string(params.TextDocument.URI))
		if err := s.notify(ctx, "textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
			URI:         params.TextDocument.URI,
			Diagnostics: []protocol.Diagnostic{},
		}); err != nil {
			return err
		}
		return reply(ctx, nil, nil)
	case "textDocument/completion":
		var params protocol.CompletionParams
		if err := decodeParams(request, &params); err != nil {
			return err
		}
		text := s.docs[string(params.TextDocument.URI)]
		return reply(ctx, s.completions(text, params.Position), nil)
	default:
		return jsonrpc2.MethodNotFoundHandler(ctx, reply, request)
	}
}

func (s *Server) publishDiagnostics(ctx context.Context, uri protocol.DocumentURI, text string) error {
	_, err := s.parser.Parse(strings.NewReader(text))
	diagnostics := []protocol.Diagnostic{}
	if err != nil {
		diagnostics = append(diagnostics, diagnosticFromError(err))
	}
	return s.notify(ctx, "textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

func diagnosticFromError(err error) protocol.Diagnostic {
	line := uint32(0)
	character := uint32(0)
	if match := dslPositionPattern.FindStringSubmatch(err.Error()); len(match) == 3 {
		if parsed, parseErr := strconv.Atoi(match[1]); parseErr == nil && parsed > 0 {
			line = uint32(parsed - 1)
		}
		if parsed, parseErr := strconv.Atoi(match[2]); parseErr == nil && parsed > 0 {
			character = uint32(parsed - 1)
		}
	}
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: line, Character: character},
			End:   protocol.Position{Line: line, Character: character + 1},
		},
		Severity: protocol.DiagnosticSeverityError,
		Source:   "bu1ld",
		Message:  err.Error(),
	}
}

func decodeParams(request jsonrpc2.Request, target any) error {
	return json.Unmarshal(request.Params(), target)
}

func (s *Server) notify(ctx context.Context, method string, params any) error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Notify(ctx, method, params)
}

func isStreamEOF(err error) bool {
	return errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF")
}

type readWriteCloser struct {
	io.Reader
	io.Writer
}

func (c readWriteCloser) Close() error {
	return nil
}

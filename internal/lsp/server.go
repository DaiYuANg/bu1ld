package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/lyonbrown4d/bu1ld/internal/dsl"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/dix"
	"github.com/samber/oops"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

var dslPositionPattern = regexp.MustCompile(`dsl:(\d+):(\d+):`)

type Server struct {
	parser *dsl.Parser
	in     io.Reader
	out    io.Writer
	conn   jsonrpc2.Conn
	docs   *mapping.Map[string, string]
	index  *completionIndex
}

func New(parser *dsl.Parser, in io.Reader, out io.Writer) *Server {
	return &Server{
		parser: parser,
		in:     in,
		out:    out,
		docs:   mapping.NewMap[string, string](),
		index:  newCompletionIndex(parser),
	}
}

func Run(ctx context.Context, in io.Reader, out io.Writer) (err error) {
	spec := NewDixApp(in, out)
	runtime, err := spec.Start(ctx)
	if err != nil {
		return oops.In("bu1ld.lsp").Wrapf(err, "start lsp dix runtime")
	}
	defer func() {
		stopCtx := context.WithoutCancel(ctx)
		if stopErr := runtime.Stop(stopCtx); stopErr != nil {
			stopErr = oops.In("bu1ld.lsp").Wrapf(stopErr, "stop lsp dix runtime")
			if err == nil {
				err = stopErr
				return
			}
			err = errors.Join(err, stopErr)
		}
	}()

	server, err := dix.ResolveAs[*Server](runtime.Container())
	if err != nil {
		return oops.In("bu1ld.lsp").Wrapf(err, "resolve lsp server")
	}
	return server.Serve(ctx)
}

func NewDixApp(in io.Reader, out io.Writer) *dix.App {
	return dix.New(
		"bu1ld lsp",
		dix.Modules(
			dix.NewModule("lsp",
				dix.Providers(
					dix.Value[io.Reader](in),
					dix.Value[io.Writer](out),
					dix.Provider0[*dsl.Parser](dsl.NewParser),
					dix.Provider3[*Server, *dsl.Parser, io.Reader, io.Writer](New),
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
	return oops.In("bu1ld.lsp").Wrapf(err, "serve LSP connection")
}

func (s *Server) handle(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	switch request.Method() {
	case "initialize":
		return replyInitialize(ctx, reply)
	case "initialized":
		return reply(ctx, nil, nil)
	case "shutdown":
		return reply(ctx, nil, nil)
	case "exit":
		return replyExit(ctx, reply, request)
	case "textDocument/didOpen":
		return s.didOpen(ctx, reply, request)
	case "textDocument/didChange":
		return s.didChange(ctx, reply, request)
	case "textDocument/didClose":
		return s.didClose(ctx, reply, request)
	case "textDocument/completion":
		return s.completion(ctx, reply, request)
	case "textDocument/hover":
		return s.hoverRequest(ctx, reply, request)
	default:
		if err := jsonrpc2.MethodNotFoundHandler(ctx, reply, request); err != nil {
			return oops.In("bu1ld.lsp").
				With("method", request.Method()).
				Wrapf(err, "handle LSP request")
		}
		return nil
	}
}

func replyInitialize(ctx context.Context, reply jsonrpc2.Replier) error {
	return reply(ctx, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncKindFull,
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", " ", "="},
			},
			HoverProvider: true,
		},
	}, nil)
}

func replyExit(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	if err := reply(ctx, nil, nil); err != nil {
		return oops.In("bu1ld.lsp").
			With("method", request.Method()).
			Wrapf(err, "reply to exit request")
	}
	return io.EOF
}

func (s *Server) didOpen(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	var params protocol.DidOpenTextDocumentParams
	if err := decodeParams(request, &params); err != nil {
		return err
	}
	s.docs.Set(string(params.TextDocument.URI), params.TextDocument.Text)
	if err := s.publishDiagnostics(ctx, params.TextDocument.URI, params.TextDocument.Text); err != nil {
		return err
	}
	return reply(ctx, nil, nil)
}

func (s *Server) didChange(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	var params protocol.DidChangeTextDocumentParams
	if err := decodeParams(request, &params); err != nil {
		return err
	}
	uri := string(params.TextDocument.URI)
	text, _ := s.docs.Get(uri)
	if len(params.ContentChanges) > 0 {
		text = params.ContentChanges[len(params.ContentChanges)-1].Text
	}
	s.docs.Set(uri, text)
	if err := s.publishDiagnostics(ctx, params.TextDocument.URI, text); err != nil {
		return err
	}
	return reply(ctx, nil, nil)
}

func (s *Server) didClose(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	var params protocol.DidCloseTextDocumentParams
	if err := decodeParams(request, &params); err != nil {
		return err
	}
	s.docs.Delete(string(params.TextDocument.URI))
	if err := s.notify(ctx, "textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	}); err != nil {
		return err
	}
	return reply(ctx, nil, nil)
}

func (s *Server) completion(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	var params protocol.CompletionParams
	if err := decodeParams(request, &params); err != nil {
		return err
	}
	text, _ := s.docs.Get(string(params.TextDocument.URI))
	return reply(ctx, s.completions(text, params.Position), nil)
}

func (s *Server) hoverRequest(ctx context.Context, reply jsonrpc2.Replier, request jsonrpc2.Request) error {
	var params protocol.HoverParams
	if err := decodeParams(request, &params); err != nil {
		return err
	}
	text, _ := s.docs.Get(string(params.TextDocument.URI))
	return reply(ctx, s.hover(text, params.Position), nil)
}

func (s *Server) publishDiagnostics(ctx context.Context, uri protocol.DocumentURI, text string) error {
	_, err := s.parser.ParseContext(ctx, strings.NewReader(text))
	diagnostics := list.NewList[protocol.Diagnostic]()
	if err != nil {
		diagnostics.Add(diagnosticFromError(err))
	}
	values := diagnostics.Values()
	if values == nil {
		values = []protocol.Diagnostic{}
	}
	return s.notify(ctx, "textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: values,
	})
}

func diagnosticFromError(err error) protocol.Diagnostic {
	line := uint32(0)
	character := uint32(0)
	if match := dslPositionPattern.FindStringSubmatch(err.Error()); len(match) == 3 {
		if parsed, parseErr := strconv.Atoi(match[1]); parseErr == nil && parsed > 0 && parsed <= math.MaxUint32 {
			line = uint32(parsed - 1)
		}
		if parsed, parseErr := strconv.Atoi(match[2]); parseErr == nil && parsed > 0 && parsed <= math.MaxUint32 {
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
	if err := json.Unmarshal(request.Params(), target); err != nil {
		return oops.In("bu1ld.lsp").
			With("method", request.Method()).
			Wrapf(err, "decode request params")
	}
	return nil
}

func (s *Server) notify(ctx context.Context, method string, params any) error {
	if s.conn == nil {
		return nil
	}
	if err := s.conn.Notify(ctx, method, params); err != nil {
		return oops.In("bu1ld.lsp").
			With("method", method).
			Wrapf(err, "send LSP notification")
	}
	return nil
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

package lsp

import (
	"bufio"
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
)

const textDocumentSyncFull = 1

var dslPositionPattern = regexp.MustCompile(`dsl:(\d+):(\d+):`)

type Server struct {
	parser *dsl.Parser
	in     io.Reader
	out    io.Writer
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
	reader := bufio.NewReader(s.in)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := readMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.handle(ctx, payload); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

func (s *Server) handle(_ context.Context, payload []byte) error {
	var request rpcRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return err
	}

	switch request.Method {
	case "initialize":
		return s.respond(request.ID, initializeResult{
			Capabilities: serverCapabilities{
				TextDocumentSync: textDocumentSyncFull,
			},
		})
	case "initialized":
		return nil
	case "shutdown":
		return s.respond(request.ID, nil)
	case "exit":
		return io.EOF
	case "textDocument/didOpen":
		var params didOpenParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return err
		}
		s.docs[params.TextDocument.URI] = params.TextDocument.Text
		return s.publishDiagnostics(params.TextDocument.URI, params.TextDocument.Text)
	case "textDocument/didChange":
		var params didChangeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return err
		}
		text := s.docs[params.TextDocument.URI]
		if len(params.ContentChanges) > 0 {
			text = params.ContentChanges[len(params.ContentChanges)-1].Text
		}
		s.docs[params.TextDocument.URI] = text
		return s.publishDiagnostics(params.TextDocument.URI, text)
	case "textDocument/didClose":
		var params didCloseParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return err
		}
		delete(s.docs, params.TextDocument.URI)
		return s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
			URI:         params.TextDocument.URI,
			Diagnostics: []diagnostic{},
		})
	default:
		if request.ID != nil {
			return s.respondError(request.ID, -32601, "method not found")
		}
		return nil
	}
}

func (s *Server) publishDiagnostics(uri string, text string) error {
	_, err := s.parser.Parse(strings.NewReader(text))
	diagnostics := []diagnostic{}
	if err != nil {
		diagnostics = append(diagnostics, diagnosticFromError(err))
	}
	return s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	})
}

func diagnosticFromError(err error) diagnostic {
	line := 0
	character := 0
	if match := dslPositionPattern.FindStringSubmatch(err.Error()); len(match) == 3 {
		if parsed, parseErr := strconv.Atoi(match[1]); parseErr == nil && parsed > 0 {
			line = parsed - 1
		}
		if parsed, parseErr := strconv.Atoi(match[2]); parseErr == nil && parsed > 0 {
			character = parsed - 1
		}
	}
	return diagnostic{
		Range: rangeValue{
			Start: position{Line: line, Character: character},
			End:   position{Line: line, Character: character + 1},
		},
		Severity: 1,
		Source:   "bu1ld",
		Message:  err.Error(),
	}
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, err
			}
			contentLength = parsed
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	return body, err
}

func (s *Server) respond(id *json.RawMessage, result any) error {
	if id == nil {
		return nil
	}
	return s.write(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) respondError(id *json.RawMessage, code int, message string) error {
	return s.write(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	})
}

func (s *Server) notify(method string, params any) error {
	return s.write(rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (s *Server) write(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
	return err
}

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
}

type serverCapabilities struct {
	TextDocumentSync int `json:"textDocumentSync"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type textDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent `json:"contentChanges"`
}

type versionedTextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type textDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type diagnostic struct {
	Range    rangeValue `json:"range"`
	Severity int        `json:"severity,omitempty"`
	Source   string     `json:"source,omitempty"`
	Message  string     `json:"message"`
}

type rangeValue struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

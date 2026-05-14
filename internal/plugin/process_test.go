package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("BU1LD_TEST_PLUGIN_PROCESS") == "1" {
		runTestPluginProcess()
		return
	}
	os.Exit(m.Run())
}

func TestProcessLoaderPerformsMetadataHandshake(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	loader := NewProcessLoader(LoadOptions{
		Env: []string{
			"BU1LD_TEST_PLUGIN_PROCESS=1",
			"BU1LD_TEST_PLUGIN_STDERR=ready",
		},
		HandshakeTimeout: time.Second,
		Stderr:           &stderr,
	})
	defer loader.Close()

	item, err := loader.Load(context.Background(), Declaration{
		Namespace: "fake",
		ID:        "org.bu1ld.fake",
		Source:    SourceLocal,
		Path:      os.Args[0],
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	metadata, err := item.Metadata()
	if err != nil {
		t.Fatalf("Metadata() error = %v", err)
	}
	if got, want := metadata.ProtocolVersion, ProtocolVersion; got != want {
		t.Fatalf("metadata protocol version = %d, want %d", got, want)
	}
	if !SupportsCapability(metadata, CapabilityExpand) {
		t.Fatalf("metadata capabilities = %#v, want expand", metadata.Capabilities)
	}

	wantPrefix := "[plugin:" + filepath.Base(os.Args[0]) + "] ready"
	if !strings.Contains(stderr.String(), wantPrefix) {
		t.Fatalf("stderr = %q, want substring %q", stderr.String(), wantPrefix)
	}
}

func TestProcessCommandUsesNodeForJavaScriptPlugin(t *testing.T) {
	t.Parallel()

	command := processCommand(filepath.Join("plugins", "node", "dist", "main.js"))
	if got := strings.TrimSuffix(filepath.Base(command.Path), ".exe"); got != "node" {
		t.Fatalf("command path = %q, want node", got)
	}
	if len(command.Args) != 2 || command.Args[1] != filepath.Join("plugins", "node", "dist", "main.js") {
		t.Fatalf("command args = %#v, want node plus plugin path", command.Args)
	}
}

func runTestPluginProcess() {
	if message := os.Getenv("BU1LD_TEST_PLUGIN_STDERR"); message != "" {
		_, _ = fmt.Fprintln(os.Stderr, message)
	}
	id, err := readTestPluginRequestID(os.Stdin)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	response := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"metadata": map[string]any{
				"id":               "org.bu1ld.fake",
				"namespace":        "fake",
				"protocol_version": ProtocolVersion,
				"capabilities":     []string{CapabilityMetadata, CapabilityExpand},
				"rules":            []any{},
			},
		},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_, _ = fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n", len(payload))
	_, _ = os.Stdout.Write(payload)
}

func readTestPluginRequestID(input io.Reader) (any, error) {
	reader := bufio.NewReader(input)
	length := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read JSON-RPC header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("parse content length: %w", err)
		}
		length = parsed
	}
	if length <= 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, fmt.Errorf("read JSON-RPC body: %w", err)
	}
	var request struct {
		ID any `json:"id"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("decode JSON-RPC request: %w", err)
	}
	return request.ID, nil
}

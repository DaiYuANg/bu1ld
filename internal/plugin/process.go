package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"bu1ld/pkg/pluginapi"

	"github.com/samber/oops"
	"go.lsp.dev/jsonrpc2"
)

type ProcessLoader struct {
	options LoadOptions
	clients []*processClient
}

func NewProcessLoader(options LoadOptions) *ProcessLoader {
	return &ProcessLoader{options: options}
}

func (l *ProcessLoader) Load(_ context.Context, declaration Declaration) (Plugin, error) {
	path, err := l.resolvePath(declaration)
	if err != nil {
		return nil, err
	}

	client, err := startProcessClient(path, l.options.Env)
	if err != nil {
		return nil, oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			With("path", path).
			Wrapf(err, "start plugin process")
	}
	l.clients = append(l.clients, client)
	return client, nil
}

type processClient struct {
	cmd    *exec.Cmd
	conn   jsonrpc2.Conn
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
}

func startProcessClient(path string, env []string) (*processClient, error) {
	cmd := processCommand(path)
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open plugin stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(pluginReadWriteCloser{
		Reader: stdout,
		Writer: stdin,
	}))
	conn.Go(ctx, jsonrpc2.MethodNotFoundHandler)
	return &processClient{
		cmd:    cmd,
		conn:   conn,
		cancel: cancel,
	}, nil
}

func (c *processClient) Metadata() (Metadata, error) {
	var result pluginapi.MetadataResult
	if err := c.call(context.Background(), pluginapi.MethodMetadata, nil, &result); err != nil {
		return Metadata{}, fmt.Errorf("call plugin metadata: %w", err)
	}
	return result.Metadata, nil
}

func (c *processClient) Expand(ctx context.Context, invocation Invocation) ([]TaskSpec, error) {
	params := pluginapi.ExpandParams{Invocation: invocation}
	var result pluginapi.ExpandResult
	if err := c.call(ctx, pluginapi.MethodExpand, params, &result); err != nil {
		return nil, fmt.Errorf("call plugin expand: %w", err)
	}
	return result.Tasks, nil
}

func (c *processClient) Configure(ctx context.Context, config PluginConfig) ([]TaskSpec, error) {
	params := pluginapi.ConfigureParams{Config: config}
	var result pluginapi.ConfigureResult
	if err := c.call(ctx, pluginapi.MethodConfigure, params, &result); err != nil {
		return nil, fmt.Errorf("call plugin configure: %w", err)
	}
	return result.Tasks, nil
}

func (c *processClient) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	params := pluginapi.ExecuteParams{Request: request}
	var result pluginapi.ExecuteResult
	if err := c.call(ctx, pluginapi.MethodExecute, params, &result); err != nil {
		return ExecuteResult{}, fmt.Errorf("call plugin execute: %w", err)
	}
	return result, nil
}

func (c *processClient) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("plugin process is closed")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := c.conn.Call(ctx, method, params, result)
	return err
}

func (c *processClient) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	cancel := c.cancel
	conn := c.conn
	process := c.cmd.Process
	cmd := c.cmd
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if process != nil {
		_ = process.Kill()
	}
	if cmd != nil {
		_ = cmd.Wait()
	}
}

type pluginReadWriteCloser struct {
	io.Reader
	io.Writer
}

func (c pluginReadWriteCloser) Close() error {
	var err error
	if closer, ok := c.Reader.(io.Closer); ok {
		err = errors.Join(err, closer.Close())
	}
	if closer, ok := c.Writer.(io.Closer); ok {
		err = errors.Join(err, closer.Close())
	}
	return err
}

func processCommand(path string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".bat" || ext == ".cmd" {
			return exec.Command("cmd", "/c", path)
		}
	}
	return exec.Command(path)
}

func mergeEnv(base []string, overrides []string) []string {
	positions := map[string]int{}
	merged := append([]string{}, base...)
	for i, item := range merged {
		if key, _, ok := strings.Cut(item, "="); ok {
			positions[key] = i
		}
	}
	for _, item := range overrides {
		key, _, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		if index, exists := positions[key]; exists {
			merged[index] = item
			continue
		}
		positions[key] = len(merged)
		merged = append(merged, item)
	}
	return merged
}

func (l *ProcessLoader) Close() {
	for _, client := range l.clients {
		client.Close()
	}
	l.clients = nil
}

func (l *ProcessLoader) ResolvePath(declaration Declaration) (string, error) {
	return l.resolvePath(normalizeDeclaration(declaration))
}

func (l *ProcessLoader) resolvePath(declaration Declaration) (string, error) {
	switch declaration.Source {
	case SourceBuiltin:
		return "", oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			New("builtin plugins are not process-backed")
	case SourceLocal:
		if declaration.Path != "" {
			return l.resolveExplicitPath(declaration, localPluginDir(l.options), true)
		}
		return l.resolveInstalledPath(localPluginDir(l.options), declaration)
	case SourceGlobal:
		if declaration.Path != "" {
			return l.resolveExplicitPath(declaration, globalPluginDir(l.options.GlobalDir), false)
		}
		return l.resolveInstalledPath(globalPluginDir(l.options.GlobalDir), declaration)
	default:
		return "", oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			Errorf("plugin source %q is not process-backed", declaration.Source)
	}
}

func (l *ProcessLoader) resolveExplicitPath(declaration Declaration, root string, projectRelative bool) (string, error) {
	candidate := declaration.Path
	if !filepath.IsAbs(candidate) {
		if projectRelative && isProjectRelative(candidate) {
			candidate = filepath.Join(projectDir(l.options.ProjectDir), candidate)
		} else {
			candidate = filepath.Join(root, candidate)
		}
	}
	resolved, ok, err := resolveExplicitManifestCandidate(candidate, declaration)
	if err != nil || ok {
		return resolved, err
	}
	return candidate, nil
}

func resolveExplicitManifestCandidate(candidate string, declaration Declaration) (string, bool, error) {
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, oops.In("bu1ld.plugins").
			With("path", candidate).
			Wrapf(err, "stat plugin path")
	}
	if info.IsDir() {
		manifestPath := filepath.Join(candidate, ManifestFileName)
		if !fileExists(manifestPath) {
			return "", false, nil
		}
		binary, err := ResolveManifestBinary(manifestPath, declaration)
		return binary, true, err
	}
	if filepath.Base(candidate) != ManifestFileName {
		return "", false, nil
	}
	binary, err := ResolveManifestBinary(candidate, declaration)
	return binary, true, err
}

func (l *ProcessLoader) resolveInstalledPath(root string, declaration Declaration) (string, error) {
	expected := pluginInstallPath(root, declaration)
	if fileExists(expected) {
		return expected, nil
	}
	path, ok, err := ResolveManifestPath(root, declaration)
	if err != nil {
		return "", err
	}
	if ok {
		return path, nil
	}
	path, ok, err = discoverInstalledPlugin(root, declaration)
	if err != nil {
		return "", err
	}
	if ok {
		return path, nil
	}
	return expected, nil
}

func pluginInstallPath(root string, declaration Declaration) string {
	id := pluginID(declaration)
	parts := []string{root, id}
	if declaration.Version != "" {
		parts = append(parts, declaration.Version)
	}
	parts = append(parts, id)
	return filepath.Join(parts...)
}

func discoverInstalledPlugin(root string, declaration Declaration) (string, bool, error) {
	patterns := pluginDiscoveryPatterns(declaration)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return "", false, oops.In("bu1ld.plugins").
				With("root", root).
				With("pattern", pattern).
				Wrapf(err, "discover installed plugins")
		}
		slices.Sort(matches)
		for _, match := range matches {
			if fileExists(match) {
				return match, true, nil
			}
		}
	}
	return "", false, nil
}

func pluginDiscoveryPatterns(declaration Declaration) []string {
	id := pluginID(declaration)
	if declaration.Version != "" {
		return []string{
			filepath.Join(id, declaration.Version, id),
			filepath.Join(id, declaration.Version, "*"),
		}
	}
	return []string{
		filepath.Join(id, "*", id),
		filepath.Join(id, "*", "*"),
		id,
		id + "*",
	}
}

func pluginID(declaration Declaration) string {
	if declaration.ID != "" {
		return declaration.ID
	}
	return declaration.Namespace
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func localPluginDir(options LoadOptions) string {
	if options.LocalDir != "" {
		return options.LocalDir
	}
	return filepath.Join(projectDir(options.ProjectDir), ".bu1ld", "plugins")
}

func LocalPluginDir(options LoadOptions) string {
	return localPluginDir(options)
}

func globalPluginDir(configured string) string {
	if configured != "" {
		return configured
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".bu1ld", "plugins")
	}
	return filepath.Join(".bu1ld", "plugins")
}

func GlobalPluginDir(configured string) string {
	return globalPluginDir(configured)
}

func projectDir(configured string) string {
	if configured != "" {
		return configured
	}
	return "."
}

func isProjectRelative(path string) bool {
	return path == "." ||
		path == ".." ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../")
}

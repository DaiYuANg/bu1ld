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
	"time"

	"bu1ld/pkg/pluginapi"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
	"go.lsp.dev/jsonrpc2"
)

const (
	defaultPluginHandshakeTimeout = 5 * time.Second
	maxPluginStderrTail           = 4096
)

type ProcessLoader struct {
	options LoadOptions
	clients *list.List[*processClient]
}

func NewProcessLoader(options LoadOptions) *ProcessLoader {
	return &ProcessLoader{
		options: options,
		clients: list.NewList[*processClient](),
	}
}

func (l *ProcessLoader) Load(ctx context.Context, declaration Declaration) (Plugin, error) {
	declaration = normalizeDeclaration(declaration)
	if declaration.Source == SourceContainer {
		client, err := startContainerClient(ctx, declaration, l.options)
		if err != nil {
			return nil, oops.In("bu1ld.plugins").
				With("namespace", declaration.Namespace).
				With("source", declaration.Source).
				With("image", declaration.Image).
				Wrapf(err, "start plugin container")
		}
		if err := client.handshake(ctx, declaration, l.options.handshakeTimeout()); err != nil {
			closeErr := client.close()
			return nil, oops.In("bu1ld.plugins").
				With("namespace", declaration.Namespace).
				With("source", declaration.Source).
				With("image", declaration.Image).
				Wrapf(client.decorateProcessError(err, closeErr), "start plugin container")
		}
		l.addClient(client)
		return client, nil
	}

	path, err := l.resolvePath(declaration)
	if err != nil {
		return nil, err
	}

	client, err := startProcessClient(path, l.options)
	if err != nil {
		return nil, oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			With("path", path).
			Wrapf(err, "start plugin process")
	}
	if err := client.handshake(ctx, declaration, l.options.handshakeTimeout()); err != nil {
		closeErr := client.close()
		return nil, oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			With("path", path).
			Wrapf(client.decorateProcessError(err, closeErr), "start plugin process")
	}
	l.addClient(client)
	return client, nil
}

func (l *ProcessLoader) addClient(client *processClient) {
	if l.clients == nil {
		l.clients = list.NewList[*processClient]()
	}
	l.clients.Add(client)
}

type processClient struct {
	cmd           *exec.Cmd
	conn          jsonrpc2.Conn
	cancel        context.CancelFunc
	stderr        *processStderr
	closeFunc     func() error
	workDirMapper *containerWorkDirMapper
	metadata      Metadata
	mu            sync.Mutex
	closed        bool
}

func startProcessClient(path string, options LoadOptions) (*processClient, error) {
	cmd := processCommand(path)
	if len(options.Env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), options.Env)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open plugin stdout: %w", err)
	}
	stderr := newProcessStderr(filepath.Base(path), options.stderrOutput())
	cmd.Stderr = stderr
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
		stderr: stderr,
	}, nil
}

func (c *processClient) Metadata() (Metadata, error) {
	c.mu.Lock()
	metadata := c.metadata
	c.mu.Unlock()
	if metadata.ProtocolVersion != 0 {
		return metadata, nil
	}
	return c.metadataCall(context.Background())
}

func (c *processClient) handshake(ctx context.Context, declaration Declaration, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	metadata, err := c.metadataCall(ctx)
	if err != nil {
		return fmt.Errorf("metadata handshake: %w", err)
	}
	if err := validateProcessMetadata(declaration, metadata); err != nil {
		return fmt.Errorf("metadata handshake: %w", err)
	}
	c.mu.Lock()
	c.metadata = metadata
	c.mu.Unlock()
	return nil
}

func (c *processClient) metadataCall(ctx context.Context) (Metadata, error) {
	var result pluginapi.MetadataResult
	if err := c.call(ctx, pluginapi.MethodMetadata, nil, &result); err != nil {
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
	if !SupportsCapability(c.metadata, CapabilityConfigure) {
		return nil, fmt.Errorf("plugin does not support configure")
	}
	params := pluginapi.ConfigureParams{Config: config}
	var result pluginapi.ConfigureResult
	if err := c.call(ctx, pluginapi.MethodConfigure, params, &result); err != nil {
		return nil, fmt.Errorf("call plugin configure: %w", err)
	}
	return result.Tasks, nil
}

func (c *processClient) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	if !SupportsCapability(c.metadata, CapabilityExecute) {
		return ExecuteResult{}, fmt.Errorf("plugin does not support execute")
	}
	if c.workDirMapper != nil {
		workDir, err := c.workDirMapper.Map(request.WorkDir)
		if err != nil {
			return ExecuteResult{}, err
		}
		request.WorkDir = workDir
	}
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
	if err != nil {
		return c.decorateProcessError(err, nil)
	}
	return nil
}

func (c *processClient) Close() {
	_ = c.close()
}

func (c *processClient) close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	cancel := c.cancel
	conn := c.conn
	cmd := c.cmd
	var process *os.Process
	if cmd != nil {
		process = cmd.Process
	}
	closeFunc := c.closeFunc
	c.mu.Unlock()

	var err error
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		err = errors.Join(err, conn.Close())
	}
	if process != nil {
		err = errors.Join(err, process.Kill())
	}
	if closeFunc != nil {
		err = errors.Join(err, closeFunc())
	}
	if cmd != nil {
		err = errors.Join(err, cmd.Wait())
	}
	return err
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
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".mjs", ".cjs":
		return exec.Command("node", path)
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".bat" || ext == ".cmd" {
			return exec.Command("cmd", "/c", path)
		}
	}
	return exec.Command(path)
}

func mergeEnv(base []string, overrides []string) []string {
	positions := mapping.NewMap[string, int]()
	merged := list.NewListWithCapacity[string](len(base)+len(overrides), base...)
	for i, item := range base {
		if key, _, ok := strings.Cut(item, "="); ok {
			positions.Set(key, i)
		}
	}
	for _, item := range overrides {
		key, _, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		if index, exists := positions.Get(key); exists {
			merged.Set(index, item)
			continue
		}
		positions.Set(key, merged.Len())
		merged.Add(item)
	}
	return merged.Values()
}

func (o LoadOptions) handshakeTimeout() time.Duration {
	if o.HandshakeTimeout > 0 {
		return o.HandshakeTimeout
	}
	return defaultPluginHandshakeTimeout
}

func (o LoadOptions) stderrOutput() io.Writer {
	if o.Stderr != nil {
		return o.Stderr
	}
	return os.Stderr
}

func validateProcessMetadata(declaration Declaration, metadata Metadata) error {
	if metadata.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("plugin protocol_version = %d, want %d", metadata.ProtocolVersion, ProtocolVersion)
	}
	if !SupportsCapability(metadata, CapabilityMetadata) {
		return fmt.Errorf("plugin metadata missing capability %q", CapabilityMetadata)
	}
	if !SupportsCapability(metadata, CapabilityExpand) {
		return fmt.Errorf("plugin metadata missing capability %q", CapabilityExpand)
	}
	if declaration.Namespace != "" && metadata.Namespace != "" && metadata.Namespace != declaration.Namespace {
		return fmt.Errorf("plugin namespace = %q, want %q", metadata.Namespace, declaration.Namespace)
	}
	if declaration.ID != "" && metadata.ID != "" && metadata.ID != declaration.ID {
		return fmt.Errorf("plugin id = %q, want %q", metadata.ID, declaration.ID)
	}
	return nil
}

func (c *processClient) decorateProcessError(err error, closeErr error) error {
	if err == nil {
		return closeErr
	}
	message := err.Error()
	if closeErr != nil && !strings.Contains(message, closeErr.Error()) {
		message += "\nprocess exit: " + closeErr.Error()
	}
	if c != nil && c.stderr != nil {
		tail := c.stderr.Tail()
		if tail != "" && !strings.Contains(message, "plugin stderr:") {
			message += "\nplugin stderr:\n" + tail
		}
	}
	return errors.New(message)
}

type processStderr struct {
	name      string
	output    io.Writer
	mu        sync.Mutex
	tail      string
	lineStart bool
}

func newProcessStderr(name string, output io.Writer) *processStderr {
	if name == "" {
		name = "plugin"
	}
	return &processStderr{name: name, output: output, lineStart: true}
}

func (w *processStderr) Write(data []byte) (int, error) {
	text := string(data)
	w.mu.Lock()
	w.tail += text
	if len(w.tail) > maxPluginStderrTail {
		w.tail = w.tail[len(w.tail)-maxPluginStderrTail:]
	}
	err := w.writePrefixedLocked(text)
	w.mu.Unlock()
	return len(data), err
}

func (w *processStderr) writePrefixedLocked(text string) error {
	if w.output == nil || text == "" {
		return nil
	}
	for text != "" {
		if w.lineStart {
			if _, err := fmt.Fprintf(w.output, "[plugin:%s] ", w.name); err != nil {
				return err
			}
			w.lineStart = false
		}
		index := strings.IndexByte(text, '\n')
		if index < 0 {
			_, err := io.WriteString(w.output, text)
			return err
		}
		if _, err := io.WriteString(w.output, text[:index+1]); err != nil {
			return err
		}
		w.lineStart = true
		text = text[index+1:]
	}
	return nil
}

func (w *processStderr) Tail() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.tail)
}

func (l *ProcessLoader) Close() {
	for _, client := range l.clients.Values() {
		client.Close()
	}
	l.clients = list.NewList[*processClient]()
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
	case SourceContainer:
		return "", oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			New("container plugins do not have a local executable path")
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
	parts := list.NewList[string](root, id)
	if declaration.Version != "" {
		parts.Add(declaration.Version)
	}
	parts.Add(id)
	return filepath.Join(parts.Values()...)
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

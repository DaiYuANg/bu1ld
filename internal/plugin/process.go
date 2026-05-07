package plugin

import (
	"context"
	"encoding/json"
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
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	encoder *json.Encoder
	decoder *json.Decoder
	mu      sync.Mutex
	nextID  int64
	closed  bool
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
	return &processClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		encoder: json.NewEncoder(stdin),
		decoder: json.NewDecoder(stdout),
	}, nil
}

func (c *processClient) Metadata() (Metadata, error) {
	var result pluginapi.MetadataResult
	if err := c.call(pluginapi.MethodMetadata, nil, &result); err != nil {
		return Metadata{}, fmt.Errorf("call plugin metadata: %w", err)
	}
	return result.Metadata, nil
}

func (c *processClient) Expand(_ context.Context, invocation Invocation) ([]TaskSpec, error) {
	params := pluginapi.ExpandParams{Invocation: invocation}
	var result pluginapi.ExpandResult
	if err := c.call(pluginapi.MethodExpand, params, &result); err != nil {
		return nil, fmt.Errorf("call plugin expand: %w", err)
	}
	return result.Tasks, nil
}

func (c *processClient) Configure(_ context.Context, config PluginConfig) ([]TaskSpec, error) {
	params := pluginapi.ConfigureParams{Config: config}
	var result pluginapi.ConfigureResult
	if err := c.call(pluginapi.MethodConfigure, params, &result); err != nil {
		return nil, fmt.Errorf("call plugin configure: %w", err)
	}
	return result.Tasks, nil
}

func (c *processClient) Execute(_ context.Context, request ExecuteRequest) (ExecuteResult, error) {
	params := pluginapi.ExecuteParams{Request: request}
	var result pluginapi.ExecuteResult
	if err := c.call(pluginapi.MethodExecute, params, &result); err != nil {
		return ExecuteResult{}, fmt.Errorf("call plugin execute: %w", err)
	}
	return result, nil
}

func (c *processClient) call(method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("plugin process is closed")
	}
	c.nextID++
	request := pluginapi.Request{
		ID:     c.nextID,
		Method: method,
	}
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal plugin params: %w", err)
		}
		request.Params = data
	}
	if err := c.encoder.Encode(request); err != nil {
		return fmt.Errorf("encode plugin request: %w", err)
	}

	var response pluginapi.Response
	if err := c.decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode plugin response: %w", err)
	}
	if response.ID != request.ID {
		return fmt.Errorf("plugin response id %d does not match request id %d", response.ID, request.ID)
	}
	if response.Error != nil {
		return errorsFromResponse(response.Error)
	}
	if result == nil {
		return nil
	}
	if len(response.Result) == 0 {
		return fmt.Errorf("plugin response result is empty")
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("decode plugin result: %w", err)
	}
	return nil
}

func errorsFromResponse(response *pluginapi.ResponseError) error {
	if response == nil || response.Message == "" {
		return fmt.Errorf("plugin returned an error")
	}
	return fmt.Errorf("%s", response.Message)
}

func (c *processClient) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	stdin := c.stdin
	process := c.cmd.Process
	cmd := c.cmd
	c.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if process != nil {
		_ = process.Kill()
	}
	if cmd != nil {
		_ = cmd.Wait()
	}
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
		if filepath.IsAbs(declaration.Path) {
			return declaration.Path, nil
		}
		if declaration.Path != "" {
			if isProjectRelative(declaration.Path) {
				return filepath.Join(projectDir(l.options.ProjectDir), declaration.Path), nil
			}
			return filepath.Join(localPluginDir(l.options), declaration.Path), nil
		}
		return l.resolveInstalledPath(localPluginDir(l.options), declaration)
	case SourceGlobal:
		if declaration.Path != "" {
			if filepath.IsAbs(declaration.Path) {
				return declaration.Path, nil
			}
			return filepath.Join(globalPluginDir(l.options.GlobalDir), declaration.Path), nil
		}
		return l.resolveInstalledPath(globalPluginDir(l.options.GlobalDir), declaration)
	default:
		return "", oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			With("source", declaration.Source).
			Errorf("plugin source %q is not process-backed", declaration.Source)
	}
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

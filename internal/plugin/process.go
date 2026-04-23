package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"bu1ld/pkg/pluginapi"

	hplugin "github.com/hashicorp/go-plugin"
)

type ProcessLoader struct {
	options LoadOptions
	clients []*hplugin.Client
}

func NewProcessLoader(options LoadOptions) *ProcessLoader {
	return &ProcessLoader{options: options}
}

func (l *ProcessLoader) Load(ctx context.Context, declaration Declaration) (Plugin, error) {
	path, err := l.resolvePath(declaration)
	if err != nil {
		return nil, err
	}

	client := hplugin.NewClient(&hplugin.ClientConfig{
		HandshakeConfig:  pluginapi.Handshake,
		Plugins:          hplugin.PluginSet{pluginapi.ProcessPluginName: pluginapi.ClientPlugin()},
		Cmd:              exec.CommandContext(ctx, path),
		AllowedProtocols: []hplugin.Protocol{hplugin.ProtocolNetRPC},
	})
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("start plugin %q: %w", declaration.Namespace, err)
	}
	raw, err := rpcClient.Dispense(pluginapi.ProcessPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispense plugin %q: %w", declaration.Namespace, err)
	}
	item, ok := raw.(Plugin)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin %q returned incompatible client %T", declaration.Namespace, raw)
	}
	l.clients = append(l.clients, client)
	return item, nil
}

func (l *ProcessLoader) Close() {
	for _, client := range l.clients {
		client.Kill()
	}
	l.clients = nil
}

func (l *ProcessLoader) ResolvePath(declaration Declaration) (string, error) {
	return l.resolvePath(normalizeDeclaration(declaration))
}

func (l *ProcessLoader) resolvePath(declaration Declaration) (string, error) {
	switch declaration.Source {
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
		return "", fmt.Errorf("%s plugin %q is not process-backed", declaration.Source, declaration.Namespace)
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
		matches, err := hplugin.Discover(pattern, root)
		if err != nil {
			return "", false, err
		}
		sort.Strings(matches)
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

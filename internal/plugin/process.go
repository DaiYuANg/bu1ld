package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		return pluginInstallPath(localPluginDir(l.options), declaration), nil
	case SourceGlobal:
		if declaration.Path != "" {
			if filepath.IsAbs(declaration.Path) {
				return declaration.Path, nil
			}
			return filepath.Join(globalPluginDir(l.options.GlobalDir), declaration.Path), nil
		}
		return pluginInstallPath(globalPluginDir(l.options.GlobalDir), declaration), nil
	default:
		return "", fmt.Errorf("%s plugin %q is not process-backed", declaration.Source, declaration.Namespace)
	}
}

func pluginInstallPath(root string, declaration Declaration) string {
	id := declaration.ID
	if id == "" {
		id = declaration.Namespace
	}
	parts := []string{root, id}
	if declaration.Version != "" {
		parts = append(parts, declaration.Version)
	}
	parts = append(parts, id)
	return filepath.Join(parts...)
}

func localPluginDir(options LoadOptions) string {
	if options.LocalDir != "" {
		return options.LocalDir
	}
	return filepath.Join(projectDir(options.ProjectDir), ".bu1ld", "plugins")
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

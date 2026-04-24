package pluginapi

import (
	"context"
	"encoding/gob"
	"fmt"
	"net/rpc"

	hplugin "github.com/hashicorp/go-plugin"
)

const ProcessPluginName = "build"

var Handshake = hplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BU1LD_PLUGIN",
	MagicCookieValue: "bu1ld-plugin-v1",
}

func init() {
	gob.Register("")
	gob.Register([]string{})
	gob.Register(map[string]any{})
}

func ClientPlugin() hplugin.Plugin {
	return &rpcPlugin{}
}

func ServeProcess(item Plugin) {
	hplugin.Serve(&hplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: hplugin.PluginSet{
			ProcessPluginName: &rpcPlugin{Impl: item},
		},
	})
}

type rpcPlugin struct {
	Impl Plugin
}

func (p *rpcPlugin) Server(*hplugin.MuxBroker) (any, error) {
	return &rpcServer{Impl: p.Impl}, nil
}

func (p *rpcPlugin) Client(_ *hplugin.MuxBroker, client *rpc.Client) (any, error) {
	return &rpcClient{client: client}, nil
}

type rpcClient struct {
	client *rpc.Client
}

func (c *rpcClient) Metadata() (Metadata, error) {
	var response metadataResponse
	if err := c.client.Call("Plugin.Metadata", metadataRequest{}, &response); err != nil {
		return Metadata{}, fmt.Errorf("call plugin metadata: %w", err)
	}
	return response.Metadata, nil
}

func (c *rpcClient) Expand(ctx context.Context, invocation Invocation) ([]TaskSpec, error) {
	var response expandResponse
	if err := c.client.Call("Plugin.Expand", expandRequest{Invocation: invocation}, &response); err != nil {
		return nil, fmt.Errorf("call plugin expand: %w", err)
	}
	return response.Tasks, nil
}

type rpcServer struct {
	Impl Plugin
}

func (s *rpcServer) Metadata(_ metadataRequest, response *metadataResponse) error {
	metadata, err := s.Impl.Metadata()
	if err != nil {
		return fmt.Errorf("read plugin metadata: %w", err)
	}
	response.Metadata = metadata
	return nil
}

func (s *rpcServer) Expand(request expandRequest, response *expandResponse) error {
	tasks, err := s.Impl.Expand(context.Background(), request.Invocation)
	if err != nil {
		return fmt.Errorf("expand plugin invocation: %w", err)
	}
	response.Tasks = tasks
	return nil
}

type metadataRequest struct{}

type metadataResponse struct {
	Metadata Metadata
}

type expandRequest struct {
	Invocation Invocation
}

type expandResponse struct {
	Tasks []TaskSpec
}

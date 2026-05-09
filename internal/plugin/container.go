package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/samber/oops"
	"go.lsp.dev/jsonrpc2"
)

const (
	defaultContainerProjectDir = "/workspace"
	containerCloseTimeout      = 10 * time.Second
)

type containerPullPolicy string

const (
	containerPullMissing containerPullPolicy = "missing"
	containerPullAlways  containerPullPolicy = "always"
	containerPullNever   containerPullPolicy = "never"
)

type containerPluginSpec struct {
	Image               string
	PullPolicy          containerPullPolicy
	HostProjectDir      string
	ContainerProjectDir string
	Config              *container.Config
	HostConfig          *container.HostConfig
	NetworkingConfig    *network.NetworkingConfig
}

func startContainerClient(ctx context.Context, declaration Declaration, options LoadOptions) (*processClient, error) {
	spec, err := newContainerPluginSpec(declaration, options)
	if err != nil {
		return nil, err
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, oops.In("bu1ld.plugins").
			With("image", spec.Image).
			Wrapf(err, "create docker client")
	}

	created, err := createPluginContainer(ctx, cli, spec)
	if err != nil {
		return nil, closeContainerStartError(err, cli, "")
	}

	attached, err := cli.ContainerAttach(ctx, created.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, closeContainerStartError(oops.In("bu1ld.plugins").
			With("image", spec.Image).
			Wrapf(err, "attach plugin container"), cli, created.ID)
	}

	stderr := newProcessStderr("container:"+declaration.Namespace, options.stderrOutput())
	stream := newContainerRPCStream(attached, stderr)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		_ = stream.Close()
		return nil, closeContainerStartError(oops.In("bu1ld.plugins").
			With("image", spec.Image).
			Wrapf(err, "start plugin container"), cli, created.ID)
	}

	connCtx, cancel := context.WithCancel(context.Background())
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(stream))
	conn.Go(connCtx, jsonrpc2.MethodNotFoundHandler)
	return &processClient{
		conn:          conn,
		cancel:        cancel,
		stderr:        stderr,
		closeFunc:     closePluginContainer(cli, created.ID, stream),
		workDirMapper: newContainerWorkDirMapper(spec.HostProjectDir, spec.ContainerProjectDir),
	}, nil
}

func newContainerPluginSpec(declaration Declaration, options LoadOptions) (containerPluginSpec, error) {
	declaration = normalizeDeclaration(declaration)
	if declaration.Image == "" {
		return containerPluginSpec{}, oops.In("bu1ld.plugins").
			With("namespace", declaration.Namespace).
			New("container plugin image is required")
	}
	pullPolicy, err := normalizeContainerPullPolicy(declaration.Pull)
	if err != nil {
		return containerPluginSpec{}, err
	}
	hostProjectDir, err := absoluteProjectDir(options.ProjectDir)
	if err != nil {
		return containerPluginSpec{}, err
	}
	containerProjectDir := normalizeContainerProjectDir(declaration.WorkDir)
	networkMode := container.NetworkMode(strings.TrimSpace(declaration.Network))
	env := mergeEnv(options.Env, []string{
		"BU1LD_PROJECT_DIR=" + containerProjectDir,
		"BU1LD_HOST_PROJECT_DIR=" + hostProjectDir,
	})
	labels := map[string]string{
		"org.bu1ld.plugin":           "true",
		"org.bu1ld.plugin.namespace": declaration.Namespace,
		"org.bu1ld.plugin.id":        declaration.ID,
		"org.bu1ld.plugin.version":   declaration.Version,
	}
	config := &container.Config{
		Image:           declaration.Image,
		AttachStdin:     true,
		AttachStdout:    true,
		AttachStderr:    true,
		OpenStdin:       true,
		StdinOnce:       false,
		Tty:             false,
		Env:             env,
		WorkingDir:      containerProjectDir,
		NetworkDisabled: networkMode.IsNone(),
		Labels:          labels,
	}
	hostConfig := &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: networkMode,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: hostProjectDir,
				Target: containerProjectDir,
			},
		},
	}
	return containerPluginSpec{
		Image:               declaration.Image,
		PullPolicy:          pullPolicy,
		HostProjectDir:      hostProjectDir,
		ContainerProjectDir: containerProjectDir,
		Config:              config,
		HostConfig:          hostConfig,
		NetworkingConfig:    &network.NetworkingConfig{},
	}, nil
}

func normalizeContainerPullPolicy(value string) (containerPullPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "missing", "if-missing", "if_missing", "if-not-present", "if_not_present":
		return containerPullMissing, nil
	case "always":
		return containerPullAlways, nil
	case "never":
		return containerPullNever, nil
	default:
		return "", oops.In("bu1ld.plugins").
			With("pull", value).
			New("container plugin pull must be missing, always, or never")
	}
}

func absoluteProjectDir(configured string) (string, error) {
	dir := projectDir(configured)
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", oops.In("bu1ld.plugins").
				With("project_dir", dir).
				Wrapf(err, "resolve project dir")
		}
		dir = abs
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", oops.In("bu1ld.plugins").
			With("project_dir", dir).
			Wrapf(err, "stat project dir")
	}
	if !info.IsDir() {
		return "", oops.In("bu1ld.plugins").
			With("project_dir", dir).
			New("project dir is not a directory")
	}
	return filepath.Clean(dir), nil
}

func normalizeContainerProjectDir(value string) string {
	trimmed := strings.TrimSpace(filepath.ToSlash(value))
	if trimmed == "" {
		return defaultContainerProjectDir
	}
	cleaned := pathpkg.Clean(trimmed)
	if cleaned == "." || cleaned == "/" {
		return defaultContainerProjectDir
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func createPluginContainer(
	ctx context.Context,
	cli *client.Client,
	spec containerPluginSpec,
) (container.CreateResponse, error) {
	if spec.PullPolicy == containerPullAlways {
		if err := pullPluginImage(ctx, cli, spec.Image); err != nil {
			return container.CreateResponse{}, err
		}
	}
	created, err := cli.ContainerCreate(ctx, spec.Config, spec.HostConfig, spec.NetworkingConfig, nil, "")
	if err == nil {
		return created, nil
	}
	if spec.PullPolicy != containerPullMissing || !isDockerImageMissing(err) {
		return container.CreateResponse{}, oops.In("bu1ld.plugins").
			With("image", spec.Image).
			Wrapf(err, "create plugin container")
	}
	if err := pullPluginImage(ctx, cli, spec.Image); err != nil {
		return container.CreateResponse{}, err
	}
	created, err = cli.ContainerCreate(ctx, spec.Config, spec.HostConfig, spec.NetworkingConfig, nil, "")
	if err != nil {
		return container.CreateResponse{}, oops.In("bu1ld.plugins").
			With("image", spec.Image).
			Wrapf(err, "create plugin container after image pull")
	}
	return created, nil
}

func pullPluginImage(ctx context.Context, cli *client.Client, name string) error {
	response, err := cli.ImagePull(ctx, name, image.PullOptions{})
	if err != nil {
		return oops.In("bu1ld.plugins").
			With("image", name).
			Wrapf(err, "pull plugin image")
	}
	defer response.Close()
	if _, err := io.Copy(io.Discard, response); err != nil {
		return oops.In("bu1ld.plugins").
			With("image", name).
			Wrapf(err, "read plugin image pull response")
	}
	return nil
}

func isDockerImageMissing(err error) bool {
	if errdefs.IsNotFound(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") ||
		strings.Contains(message, "not found")
}

func closeContainerStartError(err error, cli *client.Client, containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), containerCloseTimeout)
	defer cancel()
	if containerID != "" {
		err = errors.Join(err, removePluginContainer(ctx, cli, containerID))
	}
	return errors.Join(err, cli.Close())
}

func closePluginContainer(cli *client.Client, containerID string, stream io.Closer) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), containerCloseTimeout)
		defer cancel()
		var err error
		if stream != nil {
			err = errors.Join(err, stream.Close())
		}
		err = errors.Join(err, removePluginContainer(ctx, cli, containerID))
		err = errors.Join(err, cli.Close())
		return err
	}
}

func removePluginContainer(ctx context.Context, cli *client.Client, containerID string) error {
	if containerID == "" {
		return nil
	}
	err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

type containerRPCStream struct {
	attach       types.HijackedResponse
	stdout       *io.PipeReader
	stdoutWriter *io.PipeWriter
	done         chan struct{}
	closeOnce    sync.Once
}

func newContainerRPCStream(attach types.HijackedResponse, stderr io.Writer) *containerRPCStream {
	stdout, stdoutWriter := io.Pipe()
	stream := &containerRPCStream{
		attach:       attach,
		stdout:       stdout,
		stdoutWriter: stdoutWriter,
		done:         make(chan struct{}),
	}
	go func() {
		_, err := stdcopy.StdCopy(stdoutWriter, stderr, attach.Reader)
		if err != nil {
			_ = stdoutWriter.CloseWithError(err)
		} else {
			_ = stdoutWriter.Close()
		}
		close(stream.done)
	}()
	return stream
}

func (s *containerRPCStream) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *containerRPCStream) Write(p []byte) (int, error) {
	return s.attach.Conn.Write(p)
}

func (s *containerRPCStream) Close() error {
	s.closeOnce.Do(func() {
		_ = s.stdout.Close()
		_ = s.stdoutWriter.Close()
		s.attach.Close()
		select {
		case <-s.done:
		case <-time.After(time.Second):
		}
	})
	return nil
}

type containerWorkDirMapper struct {
	hostRoot      string
	containerRoot string
}

func newContainerWorkDirMapper(hostRoot, containerRoot string) *containerWorkDirMapper {
	return &containerWorkDirMapper{
		hostRoot:      filepath.Clean(hostRoot),
		containerRoot: containerRoot,
	}
}

func (m *containerWorkDirMapper) Map(workDir string) (string, error) {
	if workDir == "" {
		return m.containerRoot, nil
	}
	hostWorkDir := workDir
	if !filepath.IsAbs(hostWorkDir) {
		hostWorkDir = filepath.Join(m.hostRoot, hostWorkDir)
	}
	hostWorkDir = filepath.Clean(hostWorkDir)
	rel, err := filepath.Rel(m.hostRoot, hostWorkDir)
	if err != nil {
		return "", fmt.Errorf("map container plugin work dir: %w", err)
	}
	if rel == "." {
		return m.containerRoot, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("container plugin work dir %q is outside project dir %q", workDir, m.hostRoot)
	}
	return pathpkg.Join(m.containerRoot, filepath.ToSlash(rel)), nil
}

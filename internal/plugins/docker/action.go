// Package docker provides built-in Docker actions.
package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	dockerbuild "github.com/docker/docker/api/types/build"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	dockerarchive "github.com/moby/go-archive"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type ImageHandler struct {
	newClient func() (*client.Client, error)
}

func NewImageHandler() *ImageHandler {
	return &ImageHandler{
		newClient: func() (*client.Client, error) {
			return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		},
	}
}

func (h *ImageHandler) Kind() string {
	return ImageActionKind
}

func (h *ImageHandler) Run(ctx context.Context, workDir string, params map[string]any, output io.Writer) (err error) {
	spec, err := imageSpecFromParams(params)
	if err != nil {
		return err
	}
	cli, err := h.newClient()
	if err != nil {
		return oops.In("bu1ld.docker").Wrapf(err, "create docker client")
	}
	defer func() {
		err = closeDocker(err, cli.Close(), "close docker client")
	}()

	contextPath := absolutePath(workDir, spec.Context)
	buildContext, err := dockerarchive.TarWithOptions(contextPath, &dockerarchive.TarOptions{})
	if err != nil {
		return oops.In("bu1ld.docker").
			With("context", contextPath).
			Wrapf(err, "archive docker build context")
	}
	defer func() {
		err = closeDocker(err, buildContext.Close(), "close docker build context")
	}()

	response, err := cli.ImageBuild(ctx, buildContext, spec.buildOptions(workDir))
	if err != nil {
		return oops.In("bu1ld.docker").
			With("context", spec.Context).
			With("dockerfile", spec.Dockerfile).
			With("tags", spec.Tags).
			Wrapf(err, "build docker image")
	}
	defer func() {
		err = closeDocker(err, response.Body.Close(), "close docker build response")
	}()
	if _, err := io.Copy(output, response.Body); err != nil {
		return oops.In("bu1ld.docker").Wrapf(err, "stream docker build output")
	}

	if spec.Push {
		for _, tag := range spec.Tags {
			if err := h.push(ctx, cli, tag, output); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *ImageHandler) push(ctx context.Context, cli *client.Client, tag string, output io.Writer) (err error) {
	response, err := cli.ImagePush(ctx, tag, dockerimage.PushOptions{})
	if err != nil {
		return oops.In("bu1ld.docker").
			With("tag", tag).
			Wrapf(err, "push docker image")
	}
	defer func() {
		err = closeDocker(err, response.Close(), "close docker push response")
	}()
	if _, err := io.Copy(output, response); err != nil {
		return oops.In("bu1ld.docker").
			With("tag", tag).
			Wrapf(err, "stream docker push output")
	}
	return nil
}

func closeDocker(err, closeErr error, message string) error {
	if closeErr == nil {
		return err
	}
	wrapped := oops.In("bu1ld.docker").Wrapf(closeErr, "%s", message)
	if err != nil {
		return errors.Join(err, wrapped)
	}
	return wrapped
}

type imageSpec struct {
	Context    string
	Dockerfile string
	Tags       []string
	BuildArgs  map[string]string
	Platforms  []string
	Target     string
	Output     string
	Push       bool
	Load       bool
}

func imageSpecFromParams(params map[string]any) (imageSpec, error) {
	spec := imageSpec{
		Context:    stringParam(params, "context"),
		Dockerfile: stringParam(params, "dockerfile"),
		Tags:       stringSliceParam(params, "tags"),
		BuildArgs:  stringMapParam(params, "build_args"),
		Platforms:  stringSliceParam(params, "platforms"),
		Target:     stringParam(params, "target"),
		Output:     stringParam(params, "output"),
		Push:       boolParam(params, "push"),
		Load:       boolParam(params, "load"),
	}
	if spec.Context == "" {
		spec.Context = "."
	}
	if spec.Dockerfile == "" {
		spec.Dockerfile = "Dockerfile"
	}
	if len(spec.Tags) == 0 {
		return imageSpec{}, oops.In("bu1ld.docker").New("docker.image tags are required")
	}
	if len(spec.Platforms) > 1 {
		return imageSpec{}, oops.In("bu1ld.docker").
			With("platforms", spec.Platforms).
			New("docker.image Go API runner supports one platform per task")
	}
	if spec.Output != "" {
		return imageSpec{}, oops.In("bu1ld.docker").
			With("output", spec.Output).
			New("docker.image output export is not supported by the Go API runner yet")
	}
	return spec, nil
}

func (s imageSpec) buildOptions(workDir string) dockerbuild.ImageBuildOptions {
	buildArgs := map[string]*string{}
	for _, key := range sortedKeys(s.BuildArgs) {
		value := s.BuildArgs[key]
		buildArgs[key] = &value
	}
	options := dockerbuild.ImageBuildOptions{
		Tags:       s.Tags,
		Remove:     true,
		Dockerfile: dockerfileInContext(workDir, s.Context, s.Dockerfile),
		BuildArgs:  buildArgs,
		Target:     s.Target,
		Version:    dockerbuild.BuilderBuildKit,
	}
	if len(s.Platforms) == 1 {
		options.Platform = s.Platforms[0]
	}
	return options
}

func dockerfileInContext(workDir, contextDir, dockerfile string) string {
	contextPath := absolutePath(workDir, contextDir)
	dockerfilePath := absolutePath(workDir, dockerfile)
	rel, err := filepath.Rel(contextPath, dockerfilePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return dockerfile
	}
	return filepath.ToSlash(rel)
}

func stringParam(params map[string]any, key string) string {
	value, ok := params[key].(string)
	if !ok {
		return ""
	}
	return value
}

func stringSliceParam(params map[string]any, key string) []string {
	switch value := params[key].(type) {
	case []string:
		return value
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			items = append(items, fmt.Sprint(item))
		}
		return items
	default:
		return nil
	}
}

func stringMapParam(params map[string]any, key string) map[string]string {
	values := map[string]string{}
	raw, ok := params[key].(map[string]any)
	if !ok {
		return values
	}
	for name, value := range raw {
		values[name] = fmt.Sprint(value)
	}
	return values
}

func boolParam(params map[string]any, key string) bool {
	value, ok := params[key].(bool)
	if !ok {
		return false
	}
	return value
}

func sortedKeys(values map[string]string) []string {
	keys := lo.Keys[string, string](values)
	slices.Sort(keys)
	return keys
}

func absolutePath(workDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workDir, path)
}

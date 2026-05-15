package plugin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestContainerPluginSpecDefaults(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	spec, err := newContainerPluginSpec(Declaration{
		Namespace: "go",
		ID:        "org.bu1ld.go",
		Source:    SourceContainer,
		Version:   "0.1.4",
		Image:     "ghcr.io/example/bu1ld-go-plugin:0.1.4",
	}, LoadOptions{
		ProjectDir: projectDir,
		Env:        []string{"BU1LD_REMOTE_CACHE__URL=http://cache.local:8080"},
	})
	if err != nil {
		t.Fatalf("newContainerPluginSpec() error = %v", err)
	}
	if got, want := spec.PullPolicy, containerPullMissing; got != want {
		t.Fatalf("pull policy = %q, want %q", got, want)
	}
	if got, want := spec.Config.Image, "ghcr.io/example/bu1ld-go-plugin:0.1.4"; got != want {
		t.Fatalf("image = %q, want %q", got, want)
	}
	if got, want := spec.ContainerProjectDir, defaultContainerProjectDir; got != want {
		t.Fatalf("container project dir = %q, want %q", got, want)
	}
	if got, want := spec.Config.WorkingDir, defaultContainerProjectDir; got != want {
		t.Fatalf("working dir = %q, want %q", got, want)
	}
	if got, want := len(spec.HostConfig.Mounts), 1; got != want {
		t.Fatalf("mount count = %d, want %d", got, want)
	}
	mount := spec.HostConfig.Mounts[0]
	if got, want := mount.Source, filepath.Clean(projectDir); got != want {
		t.Fatalf("mount source = %q, want %q", got, want)
	}
	if got, want := mount.Target, defaultContainerProjectDir; got != want {
		t.Fatalf("mount target = %q, want %q", got, want)
	}
	if got, want := spec.HostConfig.NetworkMode, container.NetworkMode(""); got != want {
		t.Fatalf("network mode = %q, want Docker default", got)
	}
	if !stringSliceContains(spec.Config.Env, "BU1LD_PROJECT_DIR="+defaultContainerProjectDir) {
		t.Fatalf("env = %v, want BU1LD_PROJECT_DIR", spec.Config.Env)
	}
	if !stringSliceContains(spec.Config.Env, "BU1LD_REMOTE_CACHE__URL=http://cache.local:8080") {
		t.Fatalf("env = %v, want remote cache env", spec.Config.Env)
	}
}

func TestContainerPluginSpecOptions(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	spec, err := newContainerPluginSpec(Declaration{
		Namespace: "java",
		Source:    SourceContainer,
		Image:     "registry.local/bu1ld-java-plugin:test",
		Pull:      "never",
		Network:   "none",
		WorkDir:   "/repo",
	}, LoadOptions{ProjectDir: projectDir})
	if err != nil {
		t.Fatalf("newContainerPluginSpec() error = %v", err)
	}
	if got, want := spec.PullPolicy, containerPullNever; got != want {
		t.Fatalf("pull policy = %q, want %q", got, want)
	}
	if got, want := spec.ContainerProjectDir, "/repo"; got != want {
		t.Fatalf("container project dir = %q, want %q", got, want)
	}
	if got, want := spec.HostConfig.NetworkMode, container.NetworkMode("none"); got != want {
		t.Fatalf("network mode = %q, want %q", got, want)
	}
	if !spec.Config.NetworkDisabled {
		t.Fatalf("NetworkDisabled = false, want true")
	}
}

func TestContainerWorkDirMapper(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mapper := newContainerWorkDirMapper(root, "/workspace")
	mapped, err := mapper.Map(filepath.Join(root, "apps", "api"))
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if got, want := mapped, "/workspace/apps/api"; got != want {
		t.Fatalf("mapped work dir = %q, want %q", got, want)
	}

	mapped, err = mapper.Map("libs/core")
	if err != nil {
		t.Fatalf("Map(relative) error = %v", err)
	}
	if got, want := mapped, "/workspace/libs/core"; got != want {
		t.Fatalf("mapped relative work dir = %q, want %q", got, want)
	}
}

func TestContainerWorkDirMapperRejectsOutsideProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mapper := newContainerWorkDirMapper(root, "/workspace")
	_, err := mapper.Map(filepath.Dir(root))
	if err == nil {
		t.Fatalf("Map() error = nil, want outside project error")
	}
	if got, want := err.Error(), "outside project dir"; !strings.Contains(got, want) {
		t.Fatalf("Map() error = %q, want substring %q", got, want)
	}
}

func TestNormalizeContainerPullPolicy(t *testing.T) {
	t.Parallel()

	for _, item := range []struct {
		value string
		want  containerPullPolicy
	}{
		{value: "", want: containerPullMissing},
		{value: "missing", want: containerPullMissing},
		{value: "if-not-present", want: containerPullMissing},
		{value: "always", want: containerPullAlways},
		{value: "never", want: containerPullNever},
	} {
		got, err := normalizeContainerPullPolicy(item.value)
		if err != nil {
			t.Fatalf("normalizeContainerPullPolicy(%q) error = %v", item.value, err)
		}
		if got != item.want {
			t.Fatalf("normalizeContainerPullPolicy(%q) = %q, want %q", item.value, got, item.want)
		}
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

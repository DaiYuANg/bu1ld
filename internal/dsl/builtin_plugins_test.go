package dsl

import (
	"strings"
	"testing"
)

func TestParserParsesDockerImageRule(t *testing.T) {
	t.Parallel()

	project, err := NewParser().Parse(strings.NewReader(`
docker.image app {
  context = "."
  dockerfile = "Dockerfile"
  tags = ["example/app:dev"]
  build_args = { VERSION = "dev" }
  load = true
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	task, ok := project.FindTask("app")
	if !ok {
		t.Fatalf("app task not found")
	}
	if got, want := task.Action.Kind, "docker.image"; got != want {
		t.Fatalf("action kind = %q, want %q", got, want)
	}
	if got, want := task.Action.Params["load"], true; got != want {
		t.Fatalf("load = %v, want %v", got, want)
	}
	if got := strings.Join(task.Action.Params["tags"].([]string), ","); got != "example/app:dev" {
		t.Fatalf("tags = %q, want example/app:dev", got)
	}
}

func TestParserParsesArchiveRules(t *testing.T) {
	t.Parallel()

	project, err := NewParser().Parse(strings.NewReader(`
archive.zip package {
  srcs = ["dist/**"]
  out = "dist/package.zip"
}

archive.tar bundle {
  srcs = ["dist/**"]
  out = "dist/package.tar.gz"
  gzip = true
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	zipTask, ok := project.FindTask("package")
	if !ok {
		t.Fatalf("package task not found")
	}
	if got, want := zipTask.Action.Kind, "archive.zip"; got != want {
		t.Fatalf("zip action kind = %q, want %q", got, want)
	}

	tarTask, ok := project.FindTask("bundle")
	if !ok {
		t.Fatalf("bundle task not found")
	}
	if got, want := tarTask.Action.Kind, "archive.tar"; got != want {
		t.Fatalf("tar action kind = %q, want %q", got, want)
	}
	if got, want := tarTask.Action.Params["gzip"], true; got != want {
		t.Fatalf("gzip = %v, want %v", got, want)
	}
}

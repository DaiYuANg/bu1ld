package dsl

import (
	"strings"
	"testing"
)

func TestParserEvaluatesMembershipAndConditionalExpressions(t *testing.T) {
	project, err := NewParser().Parse(strings.NewReader(`
task package {
  let formats = ["zip", "tar"]
  outputs = "zip" in formats ? ["dist/package.zip"] : ["dist/package.tgz"]
}

task image_marker {
  let images = { app = "example/app:dev" }
  outputs = "app" in images ? ["dist/image.txt"] : []
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	packageTask, ok := project.FindTask("package")
	if !ok {
		t.Fatalf("package task not found")
	}
	if got, want := strings.Join(packageTask.Outputs.Values(), ","), "dist/package.zip"; got != want {
		t.Fatalf("package outputs = %q, want %q", got, want)
	}

	imageTask, ok := project.FindTask("image_marker")
	if !ok {
		t.Fatalf("image_marker task not found")
	}
	if got, want := strings.Join(imageTask.Outputs.Values(), ","), "dist/image.txt"; got != want {
		t.Fatalf("image_marker outputs = %q, want %q", got, want)
	}
}

func TestParserEvaluatesFilteredLoops(t *testing.T) {
	project, err := NewParser().Parse(strings.NewReader(`
task test_cmd {
  let packages = ["./...", "./cmd/...", "./internal/..."]
  for pkg in packages where pkg != "./..." {
    command = ["go", "test", pkg]
    break
  }
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	task, ok := project.FindTask("test_cmd")
	if !ok {
		t.Fatalf("test_cmd task not found")
	}
	if got, want := strings.Join(task.Command.Values(), " "), "go test ./cmd/..."; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}

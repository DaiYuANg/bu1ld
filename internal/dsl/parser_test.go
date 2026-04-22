package dsl

import (
	"strings"
	"testing"
)

func TestParserParsesTasks(t *testing.T) {
	t.Parallel()

	project, err := NewParser().Parse(strings.NewReader(`
task test {
  inputs = ["go.mod", "**/*.go"]
  outputs = []
  command = ["go", "test", "./..."]
}

task build {
  deps = [test]
  command = ["go", "build", "./cmd/cli"]
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got := len(project.TaskNames()); got != 2 {
		t.Fatalf("task count = %d, want 2", got)
	}

	task, ok := project.FindTask("build")
	if !ok {
		t.Fatalf("build task not found")
	}
	if got, want := task.Deps.Values()[0], "test"; got != want {
		t.Fatalf("dep = %q, want %q", got, want)
	}
}

func TestParserParsesDeclarationsAndRules(t *testing.T) {
	t.Setenv("GO_VERSION", "1.26.2")

	project, err := NewParser().Parse(strings.NewReader(`
workspace {
  name = "sample"
  default = build
}

plugin go {
  source = builtin
  id = "builtin.go"
}

toolchain go {
  version = $(env("GO_VERSION", "dev"))
  settings = { mode = "module", platform = $(os + "/" + arch) }
}

go.test test {
  packages = ["./..."]
}

go.binary build {
  deps = [test]
  main = "./cmd/cli"
  out = $("dist/" + target)
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	testTask, ok := project.FindTask("test")
	if !ok {
		t.Fatalf("test task not found")
	}
	if got, want := strings.Join(testTask.Command.Values(), " "), "go test ./..."; got != want {
		t.Fatalf("test command = %q, want %q", got, want)
	}

	buildTask, ok := project.FindTask("build")
	if !ok {
		t.Fatalf("build task not found")
	}
	if got, want := strings.Join(buildTask.Command.Values(), " "), "go build -o dist/build ./cmd/cli"; got != want {
		t.Fatalf("build command = %q, want %q", got, want)
	}
}

func TestParserEvaluatesExpressions(t *testing.T) {
	t.Setenv("BU1LD_TEST_INPUT", "**/*.go")

	project, err := NewParser().Parse(strings.NewReader(`
task build-cli {
  inputs = list("go.mod", env("BU1LD_TEST_INPUT"))
  outputs = [concat("dist/bu1ld-", os(), "-", arch())]
  command = ["go", "build", "./cmd/cli"]
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	task, ok := project.FindTask("build-cli")
	if !ok {
		t.Fatalf("build-cli task not found")
	}
	if got, want := task.Inputs.Values(), []string{"go.mod", "**/*.go"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("inputs = %v, want %v", got, want)
	}
}

func TestParserEvaluatesScriptExpressions(t *testing.T) {
	project, err := NewParser().Parse(strings.NewReader(`
task build {
  inputs = $(list("go.mod", "go.sum"))
  outputs = [$("dist/" + "bu1ld")]
  command = ["go", "build", "-o", $("dist/" + "bu1ld"), "./cmd/cli"]
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	task, ok := project.FindTask("build")
	if !ok {
		t.Fatalf("build task not found")
	}
	if got, want := strings.Join(task.Outputs.Values(), ","), "dist/bu1ld"; got != want {
		t.Fatalf("outputs = %q, want %q", got, want)
	}
}

func TestParserRejectsDuplicateTasks(t *testing.T) {
	t.Parallel()

	_, err := NewParser().Parse(strings.NewReader(`
task build {}
task build {}
`))
	if err == nil {
		t.Fatalf("Parse() error = nil, want duplicate task error")
	}
}

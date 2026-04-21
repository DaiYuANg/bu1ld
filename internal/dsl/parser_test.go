package dsl

import (
	"strings"
	"testing"
)

func TestParserParsesTasks(t *testing.T) {
	t.Parallel()

	project, err := NewParser().Parse(strings.NewReader(`
task "test" {
  inputs = ["go.mod", "**/*.go"]
  outputs = []
  command = ["go", "test", "./..."]
}

task "build" {
  deps = ["test"]
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

func TestParserEvaluatesExpressions(t *testing.T) {
	t.Setenv("BU1LD_TEST_INPUT", "**/*.go")

	project, err := NewParser().Parse(strings.NewReader(`
task concat("build", "-cli") {
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

func TestParserRejectsDuplicateTasks(t *testing.T) {
	t.Parallel()

	_, err := NewParser().Parse(strings.NewReader(`
task "build" {}
task "build" {}
`))
	if err == nil {
		t.Fatalf("Parse() error = nil, want duplicate task error")
	}
}

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
  default = package
}

plugin archive {
  source = builtin
  id = "builtin.archive"
}

toolchain go {
  version = env("GO_VERSION", "dev")
  settings = { mode = "module", platform = os + "/" + arch }
}

task compile {
  command = ["go", "build", "./cmd/cli"]
}

archive.zip package {
  deps = [compile]
  srcs = ["dist/**"]
  out = "dist/package.zip"
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	compileTask, ok := project.FindTask("compile")
	if !ok {
		t.Fatalf("compile task not found")
	}
	if got, want := strings.Join(compileTask.Command.Values(), " "), "go build ./cmd/cli"; got != want {
		t.Fatalf("compile command = %q, want %q", got, want)
	}

	packageTask, ok := project.FindTask("package")
	if !ok {
		t.Fatalf("package task not found")
	}
	if got, want := packageTask.Action.Kind, "archive.zip"; got != want {
		t.Fatalf("package action = %q, want %q", got, want)
	}
	if got, want := strings.Join(packageTask.Outputs.Values(), " "), "dist/package.zip"; got != want {
		t.Fatalf("package outputs = %q, want %q", got, want)
	}
}

func TestParserEvaluatesExpressions(t *testing.T) {
	t.Setenv("BU1LD_TEST_INPUT", "**/*.go")

	project, err := NewParser().Parse(strings.NewReader(`
task build_cli {
  inputs = ["go.mod", env("BU1LD_TEST_INPUT")]
  outputs = ["dist/bu1ld-" + os + "-" + arch]
  command = ["go", "build", "./cmd/cli"]
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	task, ok := project.FindTask("build_cli")
	if !ok {
		t.Fatalf("build_cli task not found")
	}
	if got, want := task.Inputs.Values(), []string{"go.mod", "**/*.go"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("inputs = %v, want %v", got, want)
	}
}

func TestParserEvaluatesScriptExpressions(t *testing.T) {
	project, err := NewParser().Parse(strings.NewReader(`
task build {
  inputs = ["go.mod", "go.sum"]
  outputs = ["dist/" + "bu1ld"]
  command = ["go", "build", "-o", "dist/" + "bu1ld", "./cmd/cli"]
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

func TestParserEvaluatesTaskTargetContext(t *testing.T) {
	project, err := NewParser().Parse(strings.NewReader(`
task package {
  outputs = ["dist/package"]
  command = ["sh", "-c", "echo package"]
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	task, ok := project.FindTask("package")
	if !ok {
		t.Fatalf("package task not found")
	}
	if got, want := strings.Join(task.Outputs.Values(), ","), "dist/package"; got != want {
		t.Fatalf("outputs = %q, want %q", got, want)
	}
	if got, want := task.Command.Values()[2], "echo package"; got != want {
		t.Fatalf("command script = %q, want %q", got, want)
	}
}

func TestParserEvaluatesTaskRunActions(t *testing.T) {
	project, err := NewParser().Parse(strings.NewReader(`
task pack {
  outputs = ["dist/pack.tgz"]
  run {
    exec("tar", "-czf", "dist/pack.tgz", "dist/bu1ld")
  }
}

task smoke {
  run {
    shell("echo smoke")
  }
}
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	pack, ok := project.FindTask("pack")
	if !ok {
		t.Fatalf("pack task not found")
	}
	if got, want := strings.Join(pack.Command.Values(), " "), "tar -czf dist/pack.tgz dist/bu1ld"; got != want {
		t.Fatalf("pack command = %q, want %q", got, want)
	}

	smoke, ok := project.FindTask("smoke")
	if !ok {
		t.Fatalf("smoke task not found")
	}
	if got, want := strings.Join(smoke.Command.Values(), " "), "sh -c echo smoke"; got != want {
		t.Fatalf("smoke command = %q, want %q", got, want)
	}
}

func TestParserRejectsCommandAndRunTogether(t *testing.T) {
	_, err := NewParser().Parse(strings.NewReader(`
task pack {
  command = ["tar"]
  run {
    exec("tar", "-czf", "dist/pack.tgz", "dist/bu1ld")
  }
}
`))
	if err == nil {
		t.Fatalf("Parse() error = nil, want command/run conflict")
	}
	if got, want := err.Error(), "task cannot define both command and run block"; !strings.Contains(got, want) {
		t.Fatalf("Parse() error = %q, want substring %q", got, want)
	}
}

func TestParserRejectsRunOutsideTask(t *testing.T) {
	_, err := NewParser().Parse(strings.NewReader(`
plugin archive {
  source = builtin
  run {
    shell("ignored")
  }
}
`))
	if err == nil {
		t.Fatalf("Parse() error = nil, want run outside task error")
	}
	if got, want := err.Error(), "does not allow nested forms in field-only body"; !strings.Contains(got, want) {
		t.Fatalf("Parse() error = %q, want substring %q", got, want)
	}
}

func TestParserRejectsInvalidShellAction(t *testing.T) {
	_, err := NewParser().Parse(strings.NewReader(`
task broken {
  run {
    shell("if then")
  }
}
`))
	if err == nil {
		t.Fatalf("Parse() error = nil, want shell syntax error")
	}
	if got, want := err.Error(), "shell syntax error"; !strings.Contains(got, want) {
		t.Fatalf("Parse() error = %q, want substring %q", got, want)
	}
}

func TestParserParsesImportStatement(t *testing.T) {
	file, err := NewParser().ParseFile(`
task build {}
`)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got, want := file.Result.HIR.Forms.Len(), 1; got != want {
		t.Fatalf("form count = %d, want %d", got, want)
	}
	form, ok := file.Result.HIR.Forms.Get(0)
	if !ok {
		t.Fatal("first form not found")
	}
	if got, want := form.Kind, "task"; got != want {
		t.Fatalf("first form kind = %q, want %q", got, want)
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

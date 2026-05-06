package dsl

import (
	"errors"
	"fmt"
	"go/token"
	"os"
	"slices"
	"strings"

	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	planocomp "github.com/arcgolabs/plano/compiler"
	planodiag "github.com/arcgolabs/plano/diag"
)

type File struct {
	Path   string
	Result planocomp.Result
}

type PluginDeclaration struct {
	Declaration buildplugin.Declaration
	Pos         token.Pos
}

type envDependency struct {
	Name  string
	Value string
}

type DiagnosticsError struct {
	FileSet     *token.FileSet
	Diagnostics planodiag.Diagnostics
}

func (e *DiagnosticsError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "plano diagnostics"
	}
	lines := make([]string, 0, len(e.Diagnostics))
	for i := range e.Diagnostics {
		item := e.Diagnostics[i]
		lines = append(lines, item.Format(e.FileSet))
	}
	return strings.Join(lines, "\n")
}

func diagnosticsError(fset *token.FileSet, diagnostics planodiag.Diagnostics) error {
	if !diagnostics.HasError() {
		return nil
	}
	return &DiagnosticsError{
		FileSet:     fset,
		Diagnostics: diagnostics,
	}
}

func firstPassDiagnosticsError(fset *token.FileSet, diagnostics planodiag.Diagnostics) error {
	filtered := make(planodiag.Diagnostics, 0, len(diagnostics))
	for i := range diagnostics {
		item := diagnostics[i]
		if shouldIgnoreFirstPassDiagnostic(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return diagnosticsError(fset, filtered)
}

func shouldIgnoreFirstPassDiagnostic(item planodiag.Diagnostic) bool {
	if item.Code != planodiag.CodeUnknownForm {
		return false
	}
	const prefix = `unknown form "`
	if !strings.HasPrefix(item.Message, prefix) || !strings.HasSuffix(item.Message, `"`) {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(item.Message, prefix), `"`)
	return strings.Contains(name, ".")
}

func dslErrorAt(fset *token.FileSet, pos token.Pos, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if fset == nil || !pos.IsValid() {
		return errors.New(message)
	}
	location := fset.Position(pos)
	if !location.IsValid() {
		return errors.New(message)
	}
	return fmt.Errorf("%s:%d:%d: %s", location.Filename, location.Line, location.Column, message)
}

type envTracker struct {
	items *mapping.Map[string, string]
}

func newEnvTracker() *envTracker {
	return &envTracker{items: mapping.NewMap[string, string]()}
}

func (t *envTracker) Lookup(name string) (string, bool) {
	value, ok := os.LookupEnv(name)
	if t != nil && name != "" {
		t.items.Set(name, value)
	}
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

func (t *envTracker) Values() []envDependency {
	if t == nil {
		return nil
	}
	envs := list.NewList[envDependency]()
	t.items.Range(func(name string, value string) bool {
		envs.Add(envDependency{Name: name, Value: value})
		return true
	})
	values := envs.Values()
	slices.SortFunc(values, func(left, right envDependency) int {
		return strings.Compare(left.Name, right.Name)
	})
	return values
}

func splitQualifiedKind(kind string) (string, string, bool) {
	namespace, rule, ok := strings.Cut(kind, ".")
	if !ok || namespace == "" || rule == "" {
		return "", "", false
	}
	return namespace, rule, true
}

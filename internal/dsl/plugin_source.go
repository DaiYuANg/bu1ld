package dsl

import (
	"fmt"
	"go/token"

	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/plano/ast"
	planofrontend "github.com/arcgolabs/plano/frontend/plano"
	"github.com/spf13/afero"
)

func RawPluginDeclarations(fs afero.Fs, path string) ([]PluginDeclaration, error) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	fset := token.NewFileSet()
	file, diagnostics := planofrontend.ParseFile(fset, path, data)
	if err := diagnosticsError(fset, diagnostics); err != nil {
		return nil, err
	}
	declarations := list.NewList[PluginDeclaration]()
	for _, stmt := range file.Statements {
		form, ok := stmt.(*ast.FormDecl)
		if !ok || form == nil || form.Head == nil || form.Head.String() != "plugin" || form.Body == nil {
			continue
		}
		namespace := ""
		if form.Label != nil {
			namespace = form.Label.Value
		}
		declaration := buildplugin.Declaration{Namespace: namespace}
		for _, item := range form.Body.Items {
			assign, ok := item.(*ast.Assignment)
			if !ok || assign == nil || assign.Name == nil || assign.Value == nil {
				continue
			}
			value, ok := pluginDeclarationStringValue(assign.Value)
			if !ok {
				continue
			}
			switch assign.Name.Name {
			case "source":
				declaration.Source = buildplugin.Source(value)
			case "id":
				declaration.ID = value
			case "version":
				declaration.Version = value
			case "path":
				declaration.Path = value
			case "image":
				declaration.Image = value
			case "pull":
				declaration.Pull = value
			case "network":
				declaration.Network = value
			case "work_dir":
				declaration.WorkDir = value
			}
		}
		declarations.Add(PluginDeclaration{
			Declaration: declaration,
			Pos:         form.Pos(),
		})
	}
	return declarations.Values(), nil
}

func pluginDeclarationStringValue(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.StringLiteral:
		return value.Value, true
	case *ast.IdentExpr:
		if value != nil && value.Name != nil {
			return value.Name.Name, true
		}
	case *ast.ParenExpr:
		if value != nil {
			return pluginDeclarationStringValue(value.X)
		}
	}
	return "", false
}

func RawPluginDeclarationsFromPath(fs afero.Fs, buildFilePath string) ([]PluginDeclaration, error) {
	path, err := cleanAbsPath(buildFilePath)
	if err != nil {
		return nil, err
	}
	return RawPluginDeclarations(fs, path)
}

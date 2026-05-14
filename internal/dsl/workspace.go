package dsl

import (
	"go/token"

	"github.com/lyonbrown4d/bu1ld/internal/build"

	"github.com/arcgolabs/collectionx/list"
	planocomp "github.com/arcgolabs/plano/compiler"
)

func WorkspacePackagePatterns(file *File) ([]string, error) {
	if file == nil || file.Result.HIR == nil {
		return nil, nil
	}
	patterns := list.NewList[string]()
	forms := file.Result.HIR.Forms.Values()
	for i := range forms {
		form := forms[i]
		if form.Kind != "workspace" {
			continue
		}
		field, ok := form.Field("packages")
		if !ok {
			continue
		}
		values, err := stringListValue(file.Result.FileSet, field)
		if err != nil {
			return nil, err
		}
		patterns.Add(values...)
	}
	return patterns.Values(), nil
}

func PackageMetadata(file *File, defaultName, dir string) (build.Package, error) {
	pkg := build.Package{
		Name: defaultName,
		Dir:  dir,
		Deps: list.NewList[string](),
	}
	if file == nil || file.Result.HIR == nil {
		return pkg, nil
	}
	forms := file.Result.HIR.Forms.Values()
	for i := range forms {
		form := forms[i]
		if form.Kind != "package" {
			continue
		}
		next, err := packageMetadataFromForm(file.Result.FileSet, form, pkg)
		if err != nil {
			return build.Package{}, err
		}
		pkg = next
	}
	return pkg, nil
}

func packageMetadataFromForm(fset *token.FileSet, form planocomp.HIRForm, fallback build.Package) (build.Package, error) {
	pkg := fallback
	for _, field := range form.Fields.Values() {
		switch field.Name {
		case "name":
			value, err := stringFieldValue(fset, field)
			if err != nil {
				return build.Package{}, err
			}
			pkg.Name = value
		case "deps":
			values, err := stringListValue(fset, field)
			if err != nil {
				return build.Package{}, err
			}
			pkg.Deps = list.NewList[string](values...)
		default:
			return build.Package{}, dslErrorAt(fset, field.Pos, "unknown package field %q", field.Name)
		}
	}
	if pkg.Name == "" {
		return build.Package{}, dslErrorAt(fset, form.Pos, "package name is required")
	}
	return pkg, nil
}

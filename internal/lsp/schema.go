package lsp

import buildplugin "bu1ld/internal/plugin"

func coreFields(kind string) []buildplugin.FieldSchema {
	switch kind {
	case "workspace":
		return []buildplugin.FieldSchema{
			{Name: "name", Type: buildplugin.FieldString},
			{Name: "default", Type: buildplugin.FieldString},
		}
	case "plugin":
		return []buildplugin.FieldSchema{
			{Name: "source", Type: buildplugin.FieldString},
			{Name: "id", Type: buildplugin.FieldString},
			{Name: "version", Type: buildplugin.FieldString},
			{Name: "path", Type: buildplugin.FieldString},
		}
	case "toolchain":
		return []buildplugin.FieldSchema{
			{Name: "version", Type: buildplugin.FieldString},
			{Name: "settings", Type: buildplugin.FieldObject},
		}
	case "task":
		return []buildplugin.FieldSchema{
			{Name: "deps", Type: buildplugin.FieldList},
			{Name: "inputs", Type: buildplugin.FieldList},
			{Name: "outputs", Type: buildplugin.FieldList},
			{Name: "command", Type: buildplugin.FieldList},
		}
	default:
		return nil
	}
}

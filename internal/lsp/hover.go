package lsp

import (
	"strconv"
	"strings"
	"unicode/utf8"

	buildplugin "bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"go.lsp.dev/protocol"
)

type hoverEntry struct {
	Signature string
	Detail    string
	Docs      string
}

func (s *Server) hover(text string, pos protocol.Position) *protocol.Hover {
	token, tokenRange, ok := hoverTokenAt(text, pos)
	if !ok || s.index == nil {
		return nil
	}

	if inside, kind := blockContext(text, pos); inside {
		if entry, found := s.index.fieldHover(kind, token); found {
			return newHover(entry, tokenRange)
		}
	}
	if entry, found := s.index.topLevelHover(token); found {
		return newHover(entry, tokenRange)
	}
	return nil
}

func (i *completionIndex) topLevelHover(label string) (hoverEntry, bool) {
	return i.topLevelHovers.Get(label)
}

func (i *completionIndex) fieldHover(kind string, label string) (hoverEntry, bool) {
	if inside, parent := runContext(kind); inside && parent != "task" {
		return hoverEntry{}, false
	}
	entries, ok := i.fieldHoversByKind.Get(kind)
	if !ok {
		schema, found := i.ruleSchema(kind)
		if !found {
			return hoverEntry{}, false
		}
		i.registerFieldHovers(kind, hoverEntriesForFields(kind, schema.Fields))
		entries, _ = i.fieldHoversByKind.Get(kind)
	}
	return entries.Get(label)
}

func newHover(entry hoverEntry, tokenRange protocol.Range) *protocol.Hover {
	lines := list.NewList[string]()
	if entry.Signature != "" {
		lines.Add("```bu1ld")
		lines.Add(entry.Signature)
		lines.Add("```")
	}
	if entry.Detail != "" {
		lines.Add(entry.Detail)
	}
	if entry.Docs != "" {
		lines.Add(entry.Docs)
	}
	if lines.Len() == 0 {
		return nil
	}
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: strings.Join(lines.Values(), "\n\n"),
		},
		Range: &tokenRange,
	}
}

func coreTopLevelHoverEntries() map[string]hoverEntry {
	return map[string]hoverEntry{
		"import": {
			Signature: `import "path"`,
			Detail:    "build file import",
			Docs:      "Loads another bu1ld file before project lowering. Glob patterns are supported.",
		},
		"workspace": {
			Signature: "workspace { ... }",
			Detail:    "workspace block",
			Docs:      "Declares workspace metadata and the default task.",
		},
		"plugin": {
			Signature: "plugin name { ... }",
			Detail:    "plugin declaration",
			Docs:      "Declares a builtin, local, or global plugin namespace.",
		},
		"toolchain": {
			Signature: "toolchain name { ... }",
			Detail:    "toolchain block",
			Docs:      "Declares toolchain version and settings.",
		},
		"task": {
			Signature: "task name { ... }",
			Detail:    "task block",
			Docs:      "Declares a custom task with dependencies, inputs, outputs, and actions.",
		},
	}
}

func hoverEntriesForFields(kind string, fields []buildplugin.FieldSchema) map[string]hoverEntry {
	entries := make(map[string]hoverEntry, len(fields))
	for _, field := range fields {
		detail := fieldTypeLabel(field.Type)
		if field.Required {
			detail += " required"
		}
		entries[field.Name] = hoverEntry{
			Signature: field.Name + " = " + fieldValueShape(field.Type),
			Detail:    detail,
			Docs:      fieldDocs(kind, field),
		}
	}
	return entries
}

func actionHoverEntries() map[string]hoverEntry {
	return map[string]hoverEntry{
		"exec": {
			Signature: `exec("command", "arg")`,
			Detail:    "action",
			Docs:      "Runs an external command without invoking a shell.",
		},
		"shell": {
			Signature: `shell("script")`,
			Detail:    "action",
			Docs:      "Runs a POSIX shell snippet. The script is syntax checked while parsing the DSL.",
		},
	}
}

func fieldTypeLabel(fieldType buildplugin.FieldType) string {
	switch fieldType {
	case buildplugin.FieldString:
		return "string"
	case buildplugin.FieldList:
		return "list"
	case buildplugin.FieldObject:
		return "object"
	case buildplugin.FieldBool:
		return "bool"
	default:
		return "unknown"
	}
}

func fieldValueShape(fieldType buildplugin.FieldType) string {
	switch fieldType {
	case buildplugin.FieldString:
		return strconv.Quote("")
	case buildplugin.FieldList:
		return "[]"
	case buildplugin.FieldObject:
		return "{}"
	case buildplugin.FieldBool:
		return "true"
	default:
		return "value"
	}
}

func fieldDocs(kind string, field buildplugin.FieldSchema) string {
	if docs, ok := coreFieldDocs(kind).Get(field.Name); ok {
		return docs
	}
	return "Field for " + kind + "."
}

func coreFieldDocs(kind string) *mapping.Map[string, string] {
	switch kind {
	case "workspace":
		return newStringMap(map[string]string{
			"name":    "Human-readable workspace name.",
			"default": "Task selected when no explicit target is provided.",
		})
	case "plugin":
		return newStringMap(map[string]string{
			"source":  "Plugin source: builtin, local, or global.",
			"id":      "Plugin identifier, for example builtin.go.",
			"version": "Plugin version used for local or global process plugins.",
			"path":    "Development path for a local process plugin binary.",
		})
	case "toolchain":
		return newStringMap(map[string]string{
			"version":  "Requested toolchain version.",
			"settings": "Toolchain-specific configuration map.",
		})
	case "task":
		return newStringMap(map[string]string{
			"deps":    "Tasks that must complete before this task runs.",
			"inputs":  "Files or globs that participate in task cache keys.",
			"outputs": "Files produced by this task.",
			"command": "Command argv used by the simple task runner path.",
		})
	default:
		return mapping.NewMap[string, string]()
	}
}

func newStringMap(entries map[string]string) *mapping.Map[string, string] {
	items := mapping.NewMap[string, string]()
	for key, value := range entries {
		items.Set(key, value)
	}
	return items
}

func hoverTokenAt(text string, pos protocol.Position) (string, protocol.Range, bool) {
	offset := offsetAt(text, pos)
	if offset < 0 {
		return "", protocol.Range{}, false
	}

	start := offset
	for start > 0 {
		ch, size := utf8.DecodeLastRuneInString(text[:start])
		if !isCompletionPrefixRune(ch) {
			break
		}
		start -= size
	}

	end := offset
	for end < len(text) {
		ch, size := utf8.DecodeRuneInString(text[end:])
		if !isCompletionPrefixRune(ch) {
			break
		}
		end += size
	}
	if start == end {
		return "", protocol.Range{}, false
	}

	return text[start:end], protocol.Range{
		Start: positionAt(text, start),
		End:   positionAt(text, end),
	}, true
}

func positionAt(text string, offset int) protocol.Position {
	line := uint32(0)
	character := uint32(0)
	if offset > len(text) {
		offset = len(text)
	}
	for index, ch := range text {
		if index >= offset {
			break
		}
		if ch == '\n' {
			line++
			character = 0
			continue
		}
		character++
	}
	return protocol.Position{Line: line, Character: character}
}

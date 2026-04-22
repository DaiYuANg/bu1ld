package lsp

import (
	"fmt"
	"sort"
	"strings"

	buildplugin "bu1ld/internal/plugin"
)

const (
	completionItemKindField   = 5
	completionItemKindModule  = 9
	completionItemKindKeyword = 14
)

func (s *Server) completions(text string, pos position) completionList {
	if inside, kind := blockContext(text, pos); inside {
		return completionList{Items: s.fieldCompletions(kind)}
	}
	return completionList{Items: s.topLevelCompletions()}
}

func (s *Server) topLevelCompletions() []completionItem {
	items := []completionItem{
		{Label: "import", Kind: completionItemKindKeyword, Detail: "import another build file", InsertText: "import \"\""},
		{Label: "workspace", Kind: completionItemKindKeyword, Detail: "workspace block", InsertText: "workspace {\n  name = \"\"\n  default = build\n}"},
		{Label: "plugin", Kind: completionItemKindKeyword, Detail: "plugin declaration", InsertText: "plugin name {\n  source = builtin\n  id = \"\"\n}"},
		{Label: "toolchain", Kind: completionItemKindKeyword, Detail: "toolchain block", InsertText: "toolchain name {\n  version = \"\"\n}"},
		{Label: "task", Kind: completionItemKindKeyword, Detail: "task block", InsertText: "task name {\n  command = []\n}"},
	}

	for _, schema := range s.schemas() {
		for _, rule := range schema.Rules {
			label := schema.Namespace + "." + rule.Name
			items = append(items, completionItem{
				Label:      label,
				Kind:       completionItemKindModule,
				Detail:     fmt.Sprintf("%s rule", schema.ID),
				InsertText: label + " name {\n}",
			})
		}
	}
	return sortedCompletions(items)
}

func (s *Server) fieldCompletions(kind string) []completionItem {
	fields := coreFields(kind)
	if len(fields) == 0 {
		if schema, ok := s.ruleSchema(kind); ok {
			fields = schema.Fields
		}
	}

	items := make([]completionItem, 0, len(fields))
	for _, field := range fields {
		detail := string(field.Type)
		if field.Required {
			detail += " required"
		}
		items = append(items, completionItem{
			Label:      field.Name,
			Kind:       completionItemKindField,
			Detail:     detail,
			InsertText: field.Name + " = ",
		})
	}
	return sortedCompletions(items)
}

func (s *Server) ruleSchema(kind string) (buildplugin.RuleSchema, bool) {
	namespace, ruleName, ok := strings.Cut(kind, ".")
	if !ok {
		return buildplugin.RuleSchema{}, false
	}
	for _, schema := range s.schemas() {
		if schema.Namespace != namespace {
			continue
		}
		for _, rule := range schema.Rules {
			if rule.Name == ruleName {
				return rule, true
			}
		}
	}
	return buildplugin.RuleSchema{}, false
}

func (s *Server) schemas() []buildplugin.Metadata {
	schemas, err := s.parser.Schemas()
	if err != nil {
		return nil
	}
	return schemas
}

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

func blockContext(text string, pos position) (bool, string) {
	offset := offsetAt(text, pos)
	if offset < 0 {
		return false, ""
	}
	before := text[:offset]
	open := strings.LastIndex(before, "{")
	close := strings.LastIndex(before, "}")
	if open == -1 || close > open {
		return false, ""
	}

	headerStart := strings.LastIndex(before[:open], "\n")
	if headerStart == -1 {
		headerStart = 0
	} else {
		headerStart++
	}
	header := strings.TrimSpace(before[headerStart:open])
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return false, ""
	}
	return true, fields[0]
}

func offsetAt(text string, pos position) int {
	line := 0
	character := 0
	for index, ch := range text {
		if line == pos.Line && character == pos.Character {
			return index
		}
		if ch == '\n' {
			line++
			character = 0
			continue
		}
		character++
	}
	if line == pos.Line && character == pos.Character {
		return len(text)
	}
	return -1
}

func sortedCompletions(items []completionItem) []completionItem {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})
	return items
}

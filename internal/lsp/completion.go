package lsp

import (
	"fmt"
	"sort"
	"strings"

	buildplugin "bu1ld/internal/plugin"

	"go.lsp.dev/protocol"
)

func (s *Server) completions(text string, pos protocol.Position) protocol.CompletionList {
	if inside, kind := blockContext(text, pos); inside {
		return protocol.CompletionList{Items: s.fieldCompletions(kind)}
	}
	return protocol.CompletionList{Items: s.topLevelCompletions()}
}

func (s *Server) topLevelCompletions() []protocol.CompletionItem {
	items := []protocol.CompletionItem{
		{Label: "import", Kind: protocol.CompletionItemKindKeyword, Detail: "import another build file", InsertText: "import \"\""},
		{Label: "workspace", Kind: protocol.CompletionItemKindKeyword, Detail: "workspace block", InsertText: "workspace {\n  name = \"\"\n  default = build\n}"},
		{Label: "plugin", Kind: protocol.CompletionItemKindKeyword, Detail: "plugin declaration", InsertText: "plugin name {\n  source = builtin\n  id = \"\"\n}"},
		{Label: "toolchain", Kind: protocol.CompletionItemKindKeyword, Detail: "toolchain block", InsertText: "toolchain name {\n  version = \"\"\n}"},
		{Label: "task", Kind: protocol.CompletionItemKindKeyword, Detail: "task block", InsertText: "task name {\n  command = []\n}"},
	}

	for _, schema := range s.schemas() {
		for _, rule := range schema.Rules {
			label := schema.Namespace + "." + rule.Name
			items = append(items, protocol.CompletionItem{
				Label:      label,
				Kind:       protocol.CompletionItemKindModule,
				Detail:     fmt.Sprintf("%s rule", schema.ID),
				InsertText: label + " name {\n}",
			})
		}
	}
	return sortedCompletions(items)
}

func (s *Server) fieldCompletions(kind string) []protocol.CompletionItem {
	if inside, parent := runContext(kind); inside && parent == "task" {
		return actionCompletions()
	} else if inside {
		return nil
	}
	fields := coreFields(kind)
	if len(fields) == 0 {
		if schema, ok := s.ruleSchema(kind); ok {
			fields = schema.Fields
		}
	}

	items := make([]protocol.CompletionItem, 0, len(fields))
	for _, field := range fields {
		detail := string(field.Type)
		if field.Required {
			detail += " required"
		}
		items = append(items, protocol.CompletionItem{
			Label:      field.Name,
			Kind:       protocol.CompletionItemKindField,
			Detail:     detail,
			InsertText: field.Name + " = ",
		})
	}
	return sortedCompletions(items)
}

func actionCompletions() []protocol.CompletionItem {
	return sortedCompletions([]protocol.CompletionItem{
		{Label: "exec", Kind: protocol.CompletionItemKindModule, Detail: "run external command", InsertText: "exec()"},
		{Label: "shell", Kind: protocol.CompletionItemKindModule, Detail: "run shell script", InsertText: "shell(\"\")"},
	})
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

func blockContext(text string, pos protocol.Position) (bool, string) {
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
	if fields[0] == "run" {
		parentBefore := before[:headerStart]
		parentOpen := strings.LastIndex(parentBefore, "{")
		parentClose := strings.LastIndex(parentBefore, "}")
		if parentOpen != -1 && parentClose < parentOpen {
			parentHeaderStart := strings.LastIndex(parentBefore[:parentOpen], "\n")
			if parentHeaderStart == -1 {
				parentHeaderStart = 0
			} else {
				parentHeaderStart++
			}
			parentHeader := strings.TrimSpace(parentBefore[parentHeaderStart:parentOpen])
			parentFields := strings.Fields(parentHeader)
			if len(parentFields) > 0 {
				return true, "run:" + parentFields[0]
			}
		}
	}
	return true, fields[0]
}

func runContext(kind string) (bool, string) {
	parent, ok := strings.CutPrefix(kind, "run:")
	return ok, parent
}

func offsetAt(text string, pos protocol.Position) int {
	line := uint32(0)
	character := uint32(0)
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

func sortedCompletions(items []protocol.CompletionItem) []protocol.CompletionItem {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})
	return items
}

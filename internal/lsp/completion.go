package lsp

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lyonbrown4d/bu1ld/internal/dsl"
	buildplugin "github.com/lyonbrown4d/bu1ld/internal/plugin"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	prefixx "github.com/arcgolabs/collectionx/prefix"
	"github.com/samber/mo"
	"go.lsp.dev/protocol"
)

type completionIndex struct {
	topLevelItems          *list.List[protocol.CompletionItem]
	topLevelTrie           *prefixx.Trie[protocol.CompletionItem]
	topLevelHovers         *mapping.Map[string, hoverEntry]
	ruleSchemasByNamespace *mapping.MultiMap[string, buildplugin.RuleSchema]
	fieldItemsByKind       *mapping.MultiMap[string, protocol.CompletionItem]
	fieldTriesByKind       *mapping.Map[string, *prefixx.Trie[protocol.CompletionItem]]
	fieldHoversByKind      *mapping.Table[string, string, hoverEntry]
}

func newCompletionIndex(parser *dsl.Parser) *completionIndex {
	index := &completionIndex{
		topLevelItems:          list.NewList[protocol.CompletionItem](),
		topLevelTrie:           prefixx.NewTrie[protocol.CompletionItem](),
		topLevelHovers:         mapping.NewMap[string, hoverEntry](),
		ruleSchemasByNamespace: mapping.NewMultiMap[string, buildplugin.RuleSchema](),
		fieldItemsByKind:       mapping.NewMultiMap[string, protocol.CompletionItem](),
		fieldTriesByKind:       mapping.NewMap[string, *prefixx.Trie[protocol.CompletionItem]](),
		fieldHoversByKind:      mapping.NewTable[string, string, hoverEntry](),
	}

	index.addTopLevelItems(coreTopLevelCompletionItems())
	index.addTopLevelHovers(coreTopLevelHoverEntries())
	for _, kind := range []string{"workspace", "plugin", "toolchain", "task"} {
		fields := coreFields(kind)
		index.registerFieldItems(kind, completionItemsForFields(fields))
		index.registerFieldHovers(kind, hoverEntriesForFields(kind, fields))
	}
	index.registerFieldItems("run:task", actionCompletionItems())
	index.registerFieldHovers("run:task", actionHoverEntries())

	schemas, err := parser.Schemas()
	if err != nil {
		return index
	}
	for _, schema := range schemas {
		for _, rule := range schema.Rules {
			index.ruleSchemasByNamespace.Put(schema.Namespace, rule)

			label := schema.Namespace + "." + rule.Name
			index.addTopLevelItems([]protocol.CompletionItem{{
				Label:      label,
				Kind:       protocol.CompletionItemKindModule,
				Detail:     schema.ID + " rule",
				InsertText: label + " name {\n}",
			}})
			index.addTopLevelHover(label, hoverEntry{
				Signature: label + " name { ... }",
				Detail:    schema.ID + " rule",
				Docs:      "Expands a plugin rule into one or more build tasks.",
			})
		}
	}

	index.topLevelItems = list.NewList[protocol.CompletionItem](sortedCompletions(index.topLevelItems.Values())...)
	return index
}

func (s *Server) completions(text string, pos protocol.Position) protocol.CompletionList {
	labelPrefix := completionPrefix(text, pos)
	if inside, kind := blockContext(text, pos); inside {
		return protocol.CompletionList{Items: s.fieldCompletions(kind, labelPrefix)}
	}
	return protocol.CompletionList{Items: s.topLevelCompletions(labelPrefix)}
}

func (s *Server) topLevelCompletions(labelPrefix string) []protocol.CompletionItem {
	if s.index == nil {
		return nil
	}
	return filteredCompletionItems(s.index.topLevelItems.Values(), s.index.topLevelTrie, labelPrefix)
}

func (s *Server) fieldCompletions(kind, labelPrefix string) []protocol.CompletionItem {
	if inside, parent := runContext(kind); inside && parent != "task" {
		return nil
	}
	if s.index == nil {
		return nil
	}
	trie, trieKnown := s.index.fieldTriesByKind.Get(kind)
	if !trieKnown {
		schema, ok := s.index.ruleSchemaOption(kind).Get()
		if !ok {
			return nil
		}
		s.index.registerFieldItems(kind, completionItemsForFields(schema.Fields))
		s.index.registerFieldHovers(kind, hoverEntriesForFields(kind, schema.Fields))
		trie, _ = s.index.fieldTriesByKind.Get(kind)
	}
	items := s.index.fieldItemsByKind.Get(kind)
	return filteredCompletionItems(items, trie, labelPrefix)
}

func coreTopLevelCompletionItems() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		{Label: "import", Kind: protocol.CompletionItemKindKeyword, Detail: "import another build file", InsertText: "import \"\""},
		{Label: "workspace", Kind: protocol.CompletionItemKindKeyword, Detail: "workspace block", InsertText: "workspace {\n  name = \"\"\n  default = build\n}"},
		{Label: "plugin", Kind: protocol.CompletionItemKindKeyword, Detail: "plugin declaration", InsertText: "plugin name {\n  source = builtin\n  id = \"\"\n}"},
		{Label: "toolchain", Kind: protocol.CompletionItemKindKeyword, Detail: "toolchain block", InsertText: "toolchain name {\n  version = \"\"\n}"},
		{Label: "task", Kind: protocol.CompletionItemKindKeyword, Detail: "task block", InsertText: "task name {\n  command = []\n}"},
	}
}

func completionItemsForFields(fields []buildplugin.FieldSchema) []protocol.CompletionItem {
	items := list.NewListWithCapacity[protocol.CompletionItem](len(fields))
	for _, field := range fields {
		detail := string(field.Type)
		if field.Required {
			detail += " required"
		}
		items.Add(protocol.CompletionItem{
			Label:      field.Name,
			Kind:       protocol.CompletionItemKindField,
			Detail:     detail,
			InsertText: field.Name + " = ",
		})
	}
	return sortedCompletions(items.Values())
}

func actionCompletionItems() []protocol.CompletionItem {
	return sortedCompletions([]protocol.CompletionItem{
		{Label: "exec", Kind: protocol.CompletionItemKindModule, Detail: "run external command", InsertText: "exec()"},
		{Label: "shell", Kind: protocol.CompletionItemKindModule, Detail: "run shell script", InsertText: "shell(\"\")"},
	})
}

func (i *completionIndex) addTopLevelItems(items []protocol.CompletionItem) {
	for index := range items {
		item := items[index]
		i.topLevelItems.Add(item)
		i.topLevelTrie.Put(item.Label, item)
	}
}

func (i *completionIndex) addTopLevelHovers(entries map[string]hoverEntry) {
	for label, entry := range entries {
		i.addTopLevelHover(label, entry)
	}
}

func (i *completionIndex) addTopLevelHover(label string, entry hoverEntry) {
	i.topLevelHovers.Set(label, entry)
}

func (i *completionIndex) registerFieldItems(kind string, items []protocol.CompletionItem) {
	sorted := sortedCompletions(items)
	trie := prefixx.NewTrie[protocol.CompletionItem]()
	for i := range sorted {
		item := sorted[i]
		trie.Put(item.Label, item)
	}
	i.fieldItemsByKind.Set(kind, sorted...)
	i.fieldTriesByKind.Set(kind, trie)
}

func (i *completionIndex) registerFieldHovers(kind string, entries map[string]hoverEntry) {
	for label, entry := range entries {
		i.fieldHoversByKind.Put(kind, label, entry)
	}
}

func (i *completionIndex) ruleSchema(kind string) (buildplugin.RuleSchema, bool) {
	return i.ruleSchemaOption(kind).Get()
}

func (i *completionIndex) ruleSchemaOption(kind string) mo.Option[buildplugin.RuleSchema] {
	namespace, ruleName, ok := strings.Cut(kind, ".")
	if !ok {
		return mo.None[buildplugin.RuleSchema]()
	}
	rules := i.ruleSchemasByNamespace.GetOption(namespace).OrEmpty()
	return list.NewList(rules...).FirstWhere(func(_ int, rule buildplugin.RuleSchema) bool {
		if rule.Name == ruleName {
			return true
		}
		return false
	})
}

func filteredCompletionItems(
	items []protocol.CompletionItem,
	trie *prefixx.Trie[protocol.CompletionItem],
	labelPrefix string,
) []protocol.CompletionItem {
	if labelPrefix == "" || trie == nil {
		return sortedCompletions(list.NewList(items...).Values())
	}
	return sortedCompletions(trie.ValuesWithPrefix(labelPrefix))
}

func blockContext(text string, pos protocol.Position) (bool, string) {
	offset := offsetAt(text, pos)
	if offset < 0 {
		return false, ""
	}
	before := text[:offset]
	open := strings.LastIndex(before, "{")
	closing := strings.LastIndex(before, "}")
	if open == -1 || closing > open {
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

func completionPrefix(text string, pos protocol.Position) string {
	offset := offsetAt(text, pos)
	if offset <= 0 {
		return ""
	}
	start := offset
	for start > 0 {
		ch, size := utf8.DecodeLastRuneInString(text[:start])
		if !isCompletionPrefixRune(ch) {
			break
		}
		start -= size
	}
	return text[start:offset]
}

func isCompletionPrefixRune(ch rune) bool {
	return ch == '.' || ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
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

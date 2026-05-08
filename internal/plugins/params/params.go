package params

import (
	"fmt"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/mo"
)

func String(values map[string]any, key string) string {
	value, ok := values[key].(string)
	return mo.TupleToOption(value, ok).OrEmpty()
}

func Bool(values map[string]any, key string) bool {
	value, ok := values[key].(bool)
	return mo.TupleToOption(value, ok).OrEmpty()
}

func StringSlice(values map[string]any, key string) []string {
	switch value := values[key].(type) {
	case []string:
		return list.NewList(value...).Values()
	case []any:
		return list.MapList[any, string](list.NewList(value...), func(_ int, item any) string {
			return fmt.Sprint(item)
		}).Values()
	default:
		return nil
	}
}

func StringMap(values map[string]any, key string) map[string]string {
	items := mapping.NewMap[string, string]()
	raw, ok := values[key].(map[string]any)
	if !ok {
		return items.All()
	}
	for name, value := range raw {
		items.Set(name, fmt.Sprint(value))
	}
	return items.All()
}

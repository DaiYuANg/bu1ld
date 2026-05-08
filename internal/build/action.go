package build

import "fmt"

func NormalizeActionParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	normalized, _ := normalizeValue(params).(map[string]any)
	return normalized
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = normalizeValue(item)
		}
		return result
	case map[interface{}]interface{}:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			name, ok := key.(string)
			if !ok {
				name = fmt.Sprint(key)
			}
			result[name] = normalizeValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = normalizeValue(item)
		}
		return result
	default:
		return value
	}
}

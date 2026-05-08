package build

import "testing"

func TestNormalizeActionParamsConvertsNestedInterfaceMaps(t *testing.T) {
	params := map[string]any{
		"namespace": "java",
		"params": map[interface{}]interface{}{
			"srcs": []any{"src/main/java/**/*.java"},
			"nested": map[interface{}]interface{}{
				"out": "build/classes/java/main",
			},
		},
	}

	normalized := NormalizeActionParams(params)
	payload, ok := normalized["params"].(map[string]any)
	if !ok {
		t.Fatalf("params type = %T, want map[string]any", normalized["params"])
	}
	if _, ok := payload["nested"].(map[string]any); !ok {
		t.Fatalf("nested type = %T, want map[string]any", payload["nested"])
	}
}

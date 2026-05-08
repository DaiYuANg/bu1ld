package params

import (
	"reflect"
	"testing"
)

func TestTypedParams(t *testing.T) {
	t.Parallel()

	values := map[string]any{
		"name":    "image",
		"enabled": true,
		"list":    []any{"linux", "amd64", 3},
		"map": map[string]any{
			"GOOS":   "linux",
			"GOARCH": "amd64",
			"CGO":    0,
		},
	}

	if got, want := String(values, "name"), "image"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got := Bool(values, "enabled"); !got {
		t.Fatalf("Bool() = false, want true")
	}
	if got, want := StringSlice(values, "list"), []string{"linux", "amd64", "3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("StringSlice() = %#v, want %#v", got, want)
	}
	if got, want := StringMap(values, "map"), map[string]string{"GOOS": "linux", "GOARCH": "amd64", "CGO": "0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("StringMap() = %#v, want %#v", got, want)
	}
}

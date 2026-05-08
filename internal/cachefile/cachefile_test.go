package cachefile

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/arcgolabs/collectionx/list"
	"github.com/spf13/afero"
)

type testPayload struct {
	Name  string   `json:"name"`
	Items []string `json:"items,omitempty"`
}

func TestMarshalRoundTripWithoutCompression(t *testing.T) {
	t.Parallel()

	encoded, err := Marshal(testPayload{
		Name:  "small",
		Items: []string{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if got := encoded[len(fileMagic)+1]; got != 0 {
		t.Fatalf("flags = %d, want 0", got)
	}

	var decoded testPayload
	if err := Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := testPayload{Name: "small", Items: []string{"a", "b", "c"}}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded = %#v, want %#v", decoded, want)
	}
}

func TestMarshalRoundTripWithCompression(t *testing.T) {
	t.Parallel()

	items := list.NewListWithCapacity[string](256)
	for index := range 256 {
		items.Add(fmt.Sprintf("very/long/path/%03d/%s", index, strings.Repeat("segment/", 8)))
	}
	values := items.Values()

	encoded, err := Marshal(testPayload{
		Name:  "large",
		Items: values,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if got := encoded[len(fileMagic)+1] & flagZstd; got == 0 {
		t.Fatalf("compression flag not set")
	}

	var decoded testPayload
	if err := Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := testPayload{Name: "large", Items: values}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded = %#v, want %#v", decoded, want)
	}
}

func TestWriteRead(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	path := "/cache/cache.bin"
	want := testPayload{Name: "disk", Items: []string{"x", "y"}}
	if err := Write(fs, path, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var got testPayload
	if err := Read(fs, path, &got); err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Read() payload = %#v, want %#v", got, want)
	}
}

package clierr

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/samber/oops"
)

func TestPrintOopsVerbose(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := oops.In("bu1ld.test").
		With("file", "build.bu1ld").
		Wrapf(errors.New("boom"), "load config")

	if printErr := Print(&out, err); printErr != nil {
		t.Fatalf("Print() error = %v", printErr)
	}

	value := out.String()
	for _, fragment := range []string{
		"Oops: load config: boom",
		"Domain: bu1ld.test",
		"Context:",
		"file: build.bu1ld",
	} {
		if !strings.Contains(value, fragment) {
			t.Fatalf("output %q missing fragment %q", value, fragment)
		}
	}
}

func TestPrintPlainError(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if printErr := Print(&out, errors.New("boom")); printErr != nil {
		t.Fatalf("Print() error = %v", printErr)
	}

	if got, want := out.String(), "boom\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

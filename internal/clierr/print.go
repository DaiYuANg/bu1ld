package clierr

import (
	"fmt"
	"io"

	"github.com/samber/oops"
)

func Print(out io.Writer, err error) error {
	if err == nil {
		return nil
	}

	if detailed, ok := oops.AsOops(err); ok {
		if _, writeErr := fmt.Fprintf(out, "%+v\n", detailed); writeErr != nil {
			return fmt.Errorf("print detailed error: %w", writeErr)
		}
		return nil
	}

	if _, writeErr := fmt.Fprintln(out, err); writeErr != nil {
		return fmt.Errorf("print error: %w", writeErr)
	}
	return nil
}

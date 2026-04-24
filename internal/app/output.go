package app

import (
	"fmt"
	"io"

	"github.com/samber/oops"
)

func writeLine(out io.Writer, message string) error {
	if _, err := fmt.Fprintln(out, message); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "write command output")
	}
	return nil
}

func writef(out io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(out, format, args...); err != nil {
		return oops.In("bu1ld.app").Wrapf(err, "write command output")
	}
	return nil
}

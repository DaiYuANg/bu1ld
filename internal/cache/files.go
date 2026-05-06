package cache

import (
	"fmt"
	"io"
	"os"

	"github.com/fxamacker/cbor/v2"
	"github.com/spf13/afero"
)

func copyFile(fs afero.Fs, src string, dst string, mode os.FileMode) (err error) {
	in, err := fs.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() {
		if closeErr := in.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", src, closeErr)
		}
	}()

	out, err := fs.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", dst, closeErr)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

var outputFilesDigestEncMode = mustOutputFilesDigestEncMode()

func mustOutputFilesDigestEncMode() cbor.EncMode {
	mode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	return mode
}

func isNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

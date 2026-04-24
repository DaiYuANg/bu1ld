package cachefile

import (
	"path/filepath"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

const (
	fileMagic            = "BU1LDC"
	fileVersion          = byte(1)
	flagZstd             = byte(1)
	compressionThreshold = 4 << 10
)

var (
	encMode     = mustEncMode()
	zstdEncoder = mustZstdEncoder()
	zstdDecoder = mustZstdDecoder()
)

func Read(fs afero.Fs, path string, target any) error {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return oops.In("bu1ld.cachefile").
			With("path", path).
			Wrapf(err, "read cache file")
	}
	if err := Unmarshal(data, target); err != nil {
		return oops.In("bu1ld.cachefile").
			With("path", path).
			Wrapf(err, "decode cache file")
	}
	return nil
}

func Write(fs afero.Fs, path string, value any) error {
	data, err := Marshal(value)
	if err != nil {
		return oops.In("bu1ld.cachefile").
			With("path", path).
			Wrapf(err, "encode cache file")
	}
	if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return oops.In("bu1ld.cachefile").
			With("path", filepath.Dir(path)).
			Wrapf(err, "create cache directory")
	}
	if err := afero.WriteFile(fs, path, data, 0o644); err != nil {
		return oops.In("bu1ld.cachefile").
			With("path", path).
			Wrapf(err, "write cache file")
	}
	return nil
}

func Marshal(value any) ([]byte, error) {
	payload, err := encMode.Marshal(value)
	if err != nil {
		return nil, oops.In("bu1ld.cachefile").Wrapf(err, "marshal cache payload")
	}

	flags := byte(0)
	if len(payload) >= compressionThreshold {
		compressed := zstdEncoder.EncodeAll(payload, nil)
		if len(compressed) < len(payload) {
			payload = compressed
			flags |= flagZstd
		}
	}

	data := make([]byte, 0, len(fileMagic)+2+len(payload))
	data = append(data, fileMagic...)
	data = append(data, fileVersion, flags)
	data = append(data, payload...)
	return data, nil
}

func Unmarshal(data []byte, target any) error {
	if len(data) < len(fileMagic)+2 || string(data[:len(fileMagic)]) != fileMagic {
		return oops.In("bu1ld.cachefile").New("invalid cache file header")
	}

	version := data[len(fileMagic)]
	if version != fileVersion {
		return oops.In("bu1ld.cachefile").
			With("version", version).
			Errorf("unsupported cache file version %d", version)
	}

	flags := data[len(fileMagic)+1]
	if flags&^flagZstd != 0 {
		return oops.In("bu1ld.cachefile").
			With("flags", flags).
			Errorf("unsupported cache file flags 0x%x", flags)
	}

	payload := data[len(fileMagic)+2:]
	if flags&flagZstd != 0 {
		var err error
		payload, err = zstdDecoder.DecodeAll(payload, nil)
		if err != nil {
			return oops.In("bu1ld.cachefile").Wrapf(err, "decompress cache payload")
		}
	}

	if err := cbor.Unmarshal(payload, target); err != nil {
		return oops.In("bu1ld.cachefile").Wrapf(err, "unmarshal cache payload")
	}
	return nil
}

func mustEncMode() cbor.EncMode {
	mode, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	return mode
}

func mustZstdEncoder() *zstd.Encoder {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		panic(err)
	}
	return encoder
}

func mustZstdDecoder() *zstd.Decoder {
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}
	return decoder
}

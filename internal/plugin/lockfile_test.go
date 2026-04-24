package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockFileRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), LockFileName)
	lock := NewLockFile([]LockedPlugin{
		{
			Source:    SourceLocal,
			Namespace: "rust",
			ID:        "org.bu1ld.rust",
			Version:   "0.1.0",
			Path:      "/plugins/rust",
			Checksum:  "sha256:abc",
		},
	})
	if err := WriteLockFile(path, lock); err != nil {
		t.Fatalf("WriteLockFile() error = %v", err)
	}

	loaded, found, err := ReadLockFile(path)
	if err != nil {
		t.Fatalf("ReadLockFile() error = %v", err)
	}
	if !found {
		t.Fatalf("ReadLockFile() found = false")
	}
	plugin, ok := loaded.Find(SourceLocal, "rust", "org.bu1ld.rust")
	if !ok {
		t.Fatalf("locked plugin not found")
	}
	if plugin.Checksum != "sha256:abc" {
		t.Fatalf("checksum = %q, want sha256:abc", plugin.Checksum)
	}
}

func TestChecksumFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "plugin")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	checksum, err := ChecksumFile(path)
	if err != nil {
		t.Fatalf("ChecksumFile() error = %v", err)
	}
	if checksum != "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("checksum = %q", checksum)
	}
}

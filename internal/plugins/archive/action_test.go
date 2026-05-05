package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestZipHandlerWritesArchive(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeTestFile(t, workDir, "dist/app.txt", "app")

	handler := NewZipHandler()
	err := handler.Run(context.Background(), workDir, map[string]any{
		"srcs": []string{"dist/**"},
		"out":  "out/app.zip",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	reader, err := zip.OpenReader(filepath.Join(workDir, "out/app.zip"))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	names := []string{}
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	if !slices.Contains(names, "dist/app.txt") {
		t.Fatalf("zip names = %v, want dist/app.txt", names)
	}
}

func TestTarHandlerWritesGzipArchive(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	writeTestFile(t, workDir, "dist/app.txt", "app")

	handler := NewTarHandler()
	err := handler.Run(context.Background(), workDir, map[string]any{
		"srcs": []string{"dist"},
		"out":  "out/app.tar.gz",
		"gzip": true,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	file, err := os.Open(filepath.Join(workDir, "out/app.tar.gz"))
	if err != nil {
		t.Fatalf("open tar.gz: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	if err != nil {
		t.Fatalf("read tar header: %v", err)
	}
	if header.Name != "dist/app.txt" {
		t.Fatalf("tar first file = %q, want dist/app.txt", header.Name)
	}
}

func writeTestFile(t *testing.T, root string, path string, contents string) {
	t.Helper()

	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/pkg/storage"
)

func TestMigratePhotoDryRunApplyAndNoOverwrite(t *testing.T) {
	ctx := context.Background()
	source, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dest, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	key := "selfie_in/2026/11/01/example.jpg"
	if err = source.Save(ctx, key, []byte("pixels")); err != nil {
		t.Fatal(err)
	}
	if status, err := migratePhoto(ctx, dest, source, key, 6, false); err != nil || status != "pending" {
		t.Fatalf("dry run: %s %v", status, err)
	}
	if _, err = dest.Open(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("dry run wrote a file")
	}
	if status, err := migratePhoto(ctx, dest, source, key, 6, true); err != nil || status != "uploaded" {
		t.Fatalf("apply: %s %v", status, err)
	}
	if status, err := migratePhoto(ctx, dest, source, key, 6, true); err != nil || status != "existing" {
		t.Fatalf("idempotent apply: %s %v", status, err)
	}
	if err = dest.Remove(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err = dest.Save(ctx, key, []byte("wrong!")); err != nil {
		t.Fatal(err)
	}
	if _, err = migratePhoto(ctx, dest, source, key, 6, true); err == nil {
		t.Fatal("different remote bytes were accepted")
	}
	data, err := readPhoto(ctx, dest, key)
	if err != nil || string(data) != "wrong!" {
		t.Fatal("migration overwrote conflicting object")
	}
	data, err = readPhoto(ctx, source, key)
	if err != nil || string(data) != "pixels" {
		t.Fatal("migration modified original")
	}
}

func TestExportOriginalKeysAndBytes(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := "selfie_in/2026/11/01/example.jpg"
	if err = store.Save(context.Background(), key, []byte("original pixels")); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = export(context.Background(), store, strings.NewReader(key+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(&output)
	header, err := reader.Next()
	if err != nil || header.Name != key || header.Mode != 0600 {
		t.Fatalf("header: %+v %v", header, err)
	}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "original pixels" {
		t.Fatalf("export changed pixels: %q %v", data, err)
	}
	if _, err = reader.Next(); err != io.EOF {
		t.Fatalf("expected EOF: %v", err)
	}
	if err = export(context.Background(), store, strings.NewReader("../secret\n"), io.Discard); err == nil {
		t.Fatal("unsafe key accepted")
	}
	if err = export(context.Background(), store, strings.NewReader("missing.jpg\n"), io.Discard); err == nil {
		t.Fatal("missing photo accepted")
	}
}

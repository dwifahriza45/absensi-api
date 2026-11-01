package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalConfinementAndPermissions(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "storage")
	s, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	outside := filepath.Join(base, "secret")
	if err = os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../secret", "a/../../secret", "/etc/passwd", ".", "", "a\\..\\secret", "a/../secret", "C:/secret"} {
		if err = s.Save(ctx, key, []byte("bad")); err == nil {
			t.Errorf("write accepted %q", key)
		}
		if f, err := s.Open(ctx, key); err == nil {
			f.Close()
			t.Errorf("read accepted %q", key)
		}
	}
	if err = os.Symlink(base, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if f, err := s.Open(ctx, "escape/secret"); err == nil {
		f.Close()
		t.Fatal("symlink escaped storage")
	}
	if err = s.Save(ctx, "escape/overwritten", []byte("bad")); err == nil {
		t.Fatal("write escaped storage through symlink")
	}
	if err = s.Save(ctx, "photos/one.png", []byte("pixels")); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, "photos/one.png", []byte("overwrite")); err == nil {
		t.Fatal("existing file overwritten")
	}
	file, err := s.Open(ctx, "photos/one.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "pixels" {
		t.Fatalf("read %q %v", data, err)
	}
	info, err := os.Stat(filepath.Join(root, "photos/one.png"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file permissions: %v %v", info, err)
	}
	if f, err := s.Open(ctx, "photos"); err == nil {
		f.Close()
		t.Fatal("directory exposed")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.Save(cancelled, "cancelled", []byte("bad")); err == nil {
		t.Fatal("ignored cancellation")
	}
}

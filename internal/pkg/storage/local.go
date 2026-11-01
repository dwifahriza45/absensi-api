package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

type Storage interface {
	Save(context.Context, string, []byte) error
	Open(context.Context, string) (io.ReadCloser, error)
	Remove(context.Context, string) error
}
type Local struct{ root *os.Root }

func NewLocal(directory string) (*Local, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("buat folder storage: %w", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Local{root: root}, nil
}
func (l *Local) Close() error { return l.root.Close() }
func validKey(key string) bool {
	return fs.ValidPath(key) && key != "." && !strings.ContainsAny(key, "\\:\x00")
}
func (l *Local) Save(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validKey(key) {
		return errors.New("storage key tidak valid")
	}
	if err := l.root.MkdirAll(path.Dir(key), 0700); err != nil {
		return err
	}
	// Exclusive creation also prevents overwriting an existing file or symlink.
	f, err := l.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = l.root.Remove(key)
	}
	return err
}
func (l *Local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validKey(key) {
		return nil, errors.New("storage key tidak valid")
	}
	f, err := l.root.Open(key)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("bukan file foto")
	}
	return f, nil
}

func (l *Local) Remove(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validKey(key) {
		return errors.New("storage key tidak valid")
	}
	err := l.root.Remove(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

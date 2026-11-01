// photoctl is an operator-only storage export/migration tool, not an HTTP route.
package main

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/haidarrasyad/absensi-api/internal/pkg/config"
	"github.com/haidarrasyad/absensi-api/internal/pkg/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxPhotoBytes = 5 << 20

func readPhoto(ctx context.Context, store storage.Storage, key string) ([]byte, error) {
	f, err := store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPhotoBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxPhotoBytes {
		return nil, fmt.Errorf("invalid photo size")
	}
	return data, nil
}

func export(ctx context.Context, store storage.Storage, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	tw := tar.NewWriter(output)
	for scanner.Scan() {
		key := scanner.Text()
		if !fs.ValidPath(key) || key == "." {
			return fmt.Errorf("invalid storage key")
		}
		data, err := readPhoto(ctx, store, key)
		if err != nil {
			return fmt.Errorf("read photo: %w", err)
		}
		if err = tw.WriteHeader(&tar.Header{Name: key, Size: int64(len(data)), Mode: 0600}); err != nil {
			return err
		}
		if _, err = tw.Write(data); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return tw.Close() // EOF blocks are written only after every object succeeded.
}

func migratePhoto(ctx context.Context, destination, source storage.Storage, key string, size int64, apply bool) (string, error) {
	data, err := readPhoto(ctx, source, key)
	if err != nil || int64(len(data)) != size {
		return "", fmt.Errorf("local photo missing or metadata mismatch")
	}
	remote, err := readPhoto(ctx, destination, key)
	status := "existing"
	if errors.Is(err, fs.ErrNotExist) {
		if !apply {
			return "pending", nil
		}
		err = destination.Save(ctx, key, data)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("object upload failed")
		}
		if err == nil {
			status = "uploaded"
		}
		remote, err = readPhoto(ctx, destination, key)
	}
	if err != nil || sha256.Sum256(data) != sha256.Sum256(remote) {
		return "", fmt.Errorf("remote verification failed; no overwrite/delete performed")
	}
	return status, nil
}

func migrate(ctx context.Context, destination storage.Storage, source *storage.Local, apply bool) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL required")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("database configuration invalid")
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, `SELECT storage_key, size_bytes FROM photos ORDER BY storage_key`)
	if err != nil {
		return fmt.Errorf("cannot query photos")
	}
	defer rows.Close()
	counts := map[string]int{"checked": 0, "uploaded": 0, "existing": 0, "pending": 0}
	for rows.Next() {
		var key string
		var size int64
		if err = rows.Scan(&key, &size); err != nil {
			return err
		}
		status, err := migratePhoto(ctx, destination, source, key, size, apply)
		if err != nil {
			return err
		}
		counts[status]++
		counts["checked"]++
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("photo query interrupted")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"checked": counts["checked"], "uploaded": counts["uploaded"], "existing": counts["existing"], "pending": counts["pending"], "applied": apply})
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: photoctl export | migrate-local [--local-dir /app/storage] [--apply]")
	}
	cfg, err := config.LoadStorage()
	if err != nil {
		return err
	}
	store, err := storage.New(cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	switch args[0] {
	case "export":
		if len(args) != 1 {
			return fmt.Errorf("export reads newline-separated storage keys from stdin")
		}
		return export(ctx, store, os.Stdin, os.Stdout)
	case "migrate-local":
		flags := flag.NewFlagSet("migrate-local", flag.ContinueOnError)
		directory := flags.String("local-dir", "/app/storage", "existing local photo directory")
		apply := flags.Bool("apply", false, "upload missing objects after validation")
		if err = flags.Parse(args[1:]); err != nil {
			return err
		}
		if cfg.Driver != "s3" {
			return fmt.Errorf("migrate-local requires STORAGE_DRIVER=s3")
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected argument")
		}
		source, err := storage.NewLocal(*directory)
		if err != nil {
			return err
		}
		defer source.Close()
		return migrate(ctx, store, source, *apply)
	default:
		return fmt.Errorf("unknown photoctl command")
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		// Never dump config, credentials or SDK request headers into stdout/tar.
		fmt.Fprintln(os.Stderr, "photoctl failed:", err)
		os.Exit(1)
	}
}

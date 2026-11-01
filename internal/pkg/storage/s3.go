package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/haidarrasyad/absensi-api/internal/pkg/config"
)

var ErrUnavailable = errors.New("object storage unavailable")

type Managed interface {
	Storage
	Close() error
}

func New(cfg config.Storage) (Managed, error) {
	switch cfg.Driver {
	case "local":
		return NewLocal(cfg.Directory)
	case "s3":
		return NewS3(cfg)
	default:
		return nil, fmt.Errorf("unknown storage driver")
	}
}

// S3 keeps the bucket private. The API streams authenticated reads; no public URL
// or credentials ever reach the browser. Local and S3 use identical relative keys.
type S3 struct {
	client         *s3.Client
	bucket, prefix string
	transport      *http.Transport
}

func NewS3(cfg config.Storage) (*S3, error) {
	if err := cfg.Validate(false); err != nil {
		return nil, err
	}
	if cfg.Driver != "s3" {
		return nil, fmt.Errorf("S3 driver required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns, transport.MaxIdleConnsPerHost = 64, 32
	transport.ResponseHeaderTimeout = 15 * time.Second
	httpClient := &http.Client{Transport: transport, Timeout: 25 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	c := s3.NewFromConfig(aws.Config{
		Region:                     cfg.Region,
		Credentials:                credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		HTTPClient:                 httpClient,
		RetryMaxAttempts:           2,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.ForcePathStyle
	})
	return &S3{client: c, bucket: cfg.Bucket, prefix: strings.Trim(cfg.Prefix, "/"), transport: transport}, nil
}

func (s *S3) key(key string) (string, error) {
	if !validKey(key) {
		return "", fmt.Errorf("storage key tidak valid")
	}
	if s.prefix != "" {
		return s.prefix + "/" + key, nil
	}
	return key, nil
}

func code(err error, codes ...string) bool {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return false
	}
	for _, c := range codes {
		if apiError.ErrorCode() == c {
			return true
		}
	}
	return false
}

func (s *S3) Save(ctx context.Context, key string, data []byte) error {
	objectKey, err := s.key(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))), ContentType: aws.String(http.DetectContentType(data)),
		CacheControl: aws.String("private, no-store"), IfNoneMatch: aws.String("*"),
	})
	if code(err, "PreconditionFailed", "ConditionalRequestConflict") {
		return fs.ErrExist
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

type cancelReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReader) Close() error { defer r.cancel(); return r.ReadCloser.Close() }

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	objectKey, err := s.key(key)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		defer cancel()
		if code(err, "NoSuchKey", "NotFound") {
			return nil, fs.ErrNotExist
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	return &cancelReader{ReadCloser: out.Body, cancel: cancel}, nil
}

func (s *S3) Remove(ctx context.Context, key string) error {
	objectKey, err := s.key(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if code(err, "NoSuchKey", "NotFound") {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *S3) Close() error { s.transport.CloseIdleConnections(); return nil }

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"gamegen/backend/internal/platform/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrReadLimitExceeded = errors.New("object exceeds configured read limit")

type Client struct {
	client       *minio.Client
	publicClient *minio.Client
}

func New(cfg config.StorageConfig) (*Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	publicClient, err := minio.New(cfg.PublicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.PublicUseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create public minio client: %w", err)
	}
	return &Client{client: client, publicClient: publicClient}, nil
}

func (client *Client) Check(ctx context.Context) error {
	_, err := client.client.ListBuckets(ctx)
	return err
}

func (client *Client) SDK() *minio.Client {
	return client.client
}

func (client *Client) PutFile(ctx context.Context, bucket, objectKey, path, contentType string) (int64, error) {
	info, err := client.client.FPutObject(ctx, bucket, objectKey, path, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return 0, fmt.Errorf("put object %s/%s: %w", bucket, objectKey, err)
	}
	return info.Size, nil
}

func (client *Client) Remove(ctx context.Context, bucket, objectKey string) error {
	if err := client.client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object %s/%s: %w", bucket, objectKey, err)
	}
	return nil
}

func (client *Client) PresignedGet(ctx context.Context, bucket, objectKey string, expires time.Duration) (*url.URL, error) {
	if expires <= 0 {
		expires = 15 * time.Minute
	}
	presigned, err := client.publicClient.PresignedGetObject(ctx, bucket, objectKey, expires, nil)
	if err != nil {
		return nil, fmt.Errorf("presign object %s/%s: %w", bucket, objectKey, err)
	}
	return presigned, nil
}

func (client *Client) ReadAll(ctx context.Context, bucket, objectKey string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	object, err := client.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s/%s: %w", bucket, objectKey, err)
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read object %s/%s: %w", bucket, objectKey, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: %s/%s", ErrReadLimitExceeded, bucket, objectKey)
	}
	return data, nil
}

func CopyToTemporaryFile(source io.Reader, bufferBytes int) (*os.File, int64, error) {
	file, err := os.CreateTemp("", "gamegen-upload-*")
	if err != nil {
		return nil, 0, fmt.Errorf("create upload staging file: %w", err)
	}
	if bufferBytes <= 0 {
		bufferBytes = 1024 * 1024
	}
	buffer := make([]byte, bufferBytes)
	written, err := io.CopyBuffer(file, source, buffer)
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, 0, fmt.Errorf("stage upload: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, 0, fmt.Errorf("rewind staged upload: %w", err)
	}
	return file, written, nil
}

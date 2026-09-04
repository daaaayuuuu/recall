package storage

import (
	"context"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"
)

func TestPresignedGetUsesPublicEndpoint(t *testing.T) {
	client, err := New(config.StorageConfig{
		Endpoint:       "minio:9000",
		PublicEndpoint: "assets.example.com",
		AccessKey:      "access-key",
		SecretKey:      "secret-key",
		Region:         "us-east-1",
		PublicUseSSL:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	presigned, err := client.PresignedGet(context.Background(), "gamegen-render-assets", "games/example.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if presigned.Scheme != "https" || presigned.Host != "assets.example.com" {
		t.Fatalf("unexpected public URL: %s", presigned)
	}
	if internal := client.SDK().EndpointURL(); internal.Scheme != "http" || internal.Host != "minio:9000" {
		t.Fatalf("unexpected internal endpoint: %s", internal)
	}
}

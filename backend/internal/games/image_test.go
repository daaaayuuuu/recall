package games

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestProcessImageNormalizesAndHashes(t *testing.T) {
	sourceImage := image.NewRGBA(image.Rect(0, 0, 3, 2))
	sourceImage.Set(1, 1, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, sourceImage); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessImage(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer processed.File.Close()
	defer func() { _ = os.Remove(processed.File.Name()) }()

	if processed.Width != 3 || processed.Height != 2 || processed.MIMEType != "image/png" || processed.SizeBytes == 0 {
		t.Fatalf("unexpected processed image: %#v", processed)
	}
}

func TestProcessImageRejectsNonImage(t *testing.T) {
	if _, err := ProcessImage(bytes.NewReader([]byte("not an image"))); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("expected invalid image error, got %v", err)
	}
}

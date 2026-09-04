package imageprocessing

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestProcessAvatarCropsAndDownsizes(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 900, 600))
	source.Set(450, 300, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	processed, err := ProcessAvatar(bytes.NewReader(encoded.Bytes()), 512)
	if err != nil {
		t.Fatal(err)
	}
	defer processed.File.Close()
	defer func() { _ = os.Remove(processed.File.Name()) }()

	if processed.Width != 512 || processed.Height != 512 || processed.MIMEType != "image/png" || processed.SizeBytes == 0 {
		t.Fatalf("unexpected processed avatar: %#v", processed)
	}
}

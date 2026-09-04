package imageprocessing

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	_ "image/jpeg"
	_ "image/png"
)

const maxDecodedPixels = 100_000_000

var ErrInvalidImage = errors.New("invalid or unsupported image")

type ProcessedImage struct {
	File           *os.File
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 [sha256.Size]byte
}

// Process normalizes supported images to PNG so metadata and active content are not retained.
func Process(source io.ReadSeeker) (ProcessedImage, error) {
	return process(source, 0, false)
}

// ProcessAvatar center-crops an image to a square and downsizes it for profile display.
func ProcessAvatar(source io.ReadSeeker, maximumDimension int) (ProcessedImage, error) {
	if maximumDimension <= 0 {
		maximumDimension = 512
	}
	return process(source, maximumDimension, true)
}

func process(source io.ReadSeeker, maximumDimension int, square bool) (ProcessedImage, error) {
	configuration, format, err := image.DecodeConfig(source)
	if err != nil || !supportedImageFormat(format) || configuration.Width <= 0 || configuration.Height <= 0 {
		return ProcessedImage{}, ErrInvalidImage
	}
	pixels := uint64(configuration.Width) * uint64(configuration.Height)
	if pixels > maxDecodedPixels {
		return ProcessedImage{}, errors.New("image dimensions exceed the safe decoding limit")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return ProcessedImage{}, fmt.Errorf("rewind image: %w", err)
	}
	decoded, decodedFormat, err := image.Decode(source)
	if err != nil || decodedFormat != format {
		return ProcessedImage{}, ErrInvalidImage
	}

	width, height := configuration.Width, configuration.Height
	output := decoded
	if square {
		side := min(width, height)
		target := min(side, maximumDimension)
		destination := image.NewRGBA(image.Rect(0, 0, target, target))
		bounds := decoded.Bounds()
		left := bounds.Min.X + (bounds.Dx()-side)/2
		top := bounds.Min.Y + (bounds.Dy()-side)/2
		draw.CatmullRom.Scale(destination, destination.Bounds(), decoded, image.Rect(left, top, left+side, top+side), draw.Over, nil)
		output = destination
		width, height = target, target
	}

	normalized, err := os.CreateTemp("", "gamegen-normalized-*.png")
	if err != nil {
		return ProcessedImage{}, fmt.Errorf("create normalized image: %w", err)
	}
	cleanupOnError := func(err error) (ProcessedImage, error) {
		_ = normalized.Close()
		_ = os.Remove(normalized.Name())
		return ProcessedImage{}, err
	}

	hasher := sha256.New()
	if err := png.Encode(io.MultiWriter(normalized, hasher), output); err != nil {
		return cleanupOnError(fmt.Errorf("normalize image: %w", err))
	}
	info, err := normalized.Stat()
	if err != nil {
		return cleanupOnError(fmt.Errorf("stat normalized image: %w", err))
	}
	if _, err := normalized.Seek(0, io.SeekStart); err != nil {
		return cleanupOnError(fmt.Errorf("rewind normalized image: %w", err))
	}
	var checksum [sha256.Size]byte
	copy(checksum[:], hasher.Sum(nil))
	return ProcessedImage{
		File: normalized, MIMEType: "image/png", Width: width,
		Height: height, SizeBytes: info.Size(), ChecksumSHA256: checksum,
	}, nil
}

func supportedImageFormat(format string) bool {
	return format == "jpeg" || format == "png" || format == "webp"
}

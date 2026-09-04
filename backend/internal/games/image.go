package games

import (
	"io"

	"gamegen/backend/internal/platform/imageprocessing"
)

var ErrInvalidImage = imageprocessing.ErrInvalidImage

type ProcessedImage = imageprocessing.ProcessedImage

func ProcessImage(source io.ReadSeeker) (ProcessedImage, error) {
	return imageprocessing.Process(source)
}

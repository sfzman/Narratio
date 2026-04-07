package image

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
)

var fallbackBackgroundColor = color.RGBA{R: 0x1a, G: 0x1a, B: 0x2e, A: 0xff}

func writeFallbackImages(artifacts artifactWriter, images []GeneratedImage) error {
	for _, item := range images {
		if !item.IsFallback {
			continue
		}
		data, err := buildFallbackJPEG(item.Width, item.Height)
		if err != nil {
			return fmt.Errorf("build fallback jpeg: %w", err)
		}
		if err := artifacts.WriteBytes(item.FilePath, data); err != nil {
			return fmt.Errorf("write fallback image: %w", err)
		}
	}

	return nil
}

func buildFallbackJPEG(width int, height int) ([]byte, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, normalizedDimension(width, defaultImageWidth), normalizedDimension(height, defaultImageHeight)))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: fallbackBackgroundColor}, image.Point{}, draw.Src)

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, canvas, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode fallback jpeg: %w", err)
	}

	return buffer.Bytes(), nil
}

func normalizedDimension(value int, fallback int) int {
	if value > 0 {
		return value
	}

	return fallback
}

package photos

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
)

const maxPixels = 12_000_000
const maxDimension = 8192
const jpegQuality = 75

type imageBuffer struct{ bytes.Buffer }

func (b *imageBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		return 0, ErrTooLarge
	}
	return b.Buffer.Write(p)
}

// normalizeImage checks dimensions before decoding, validates the full image,
// and rewrites pixels only, stripping metadata and appended payloads.
func normalizeImage(data []byte) ([]byte, string, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxDimension || cfg.Height > maxDimension || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, "", "", ErrInvalidImage
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return nil, "", "", ErrInvalidImage
	}
	output := &imageBuffer{}
	contentType, extension := "image/jpeg", "jpg"
	if format == "png" {
		contentType, extension = "image/png", "png"
		err = png.Encode(output, decoded)
	} else {
		err = jpeg.Encode(output, decoded, &jpeg.Options{Quality: jpegQuality})
	}
	if err != nil {
		return nil, "", "", err
	}
	return output.Bytes(), contentType, extension, nil
}

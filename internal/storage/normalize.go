package storage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"path"
	"strings"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const (
	MaxUploadBytes      = 4 << 20
	MaxLegacyImageBytes = 10 << 20
	MaxStoredImageBytes = 10 << 20
	maxImageDimension   = 8192
	maxImagePixels      = 16_000_000
)

var ErrInvalidImage = errors.New("invalid or unsupported image")
var ErrImageTooLarge = errors.New("image exceeds size or dimension limits")
var ErrInvalidImageKey = errors.New("image storage key is invalid or does not belong to this record")

type NormalizedImage struct {
	Data        []byte
	ContentType string
}

func SupportedImageType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// NormalizeImage fully decodes then re-encodes the pixels, discarding metadata,
// trailing bytes, active content, and animation. HTTP uploads have a smaller
// bound than the administrative migration, which shares this validation.
func NormalizeImage(data []byte, contentType string) (*NormalizedImage, error) {
	if len(data) == 0 || !SupportedImageType(contentType) {
		return nil, ErrInvalidImage
	}
	if len(data) > MaxLegacyImageBytes {
		return nil, ErrImageTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalidImage
	}
	expected := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}[format]
	if expected == "" || expected != contentType {
		return nil, ErrInvalidImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageDimension || cfg.Height > maxImageDimension || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, ErrImageTooLarge
	}
	decoded, actualFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || actualFormat != format {
		return nil, ErrInvalidImage
	}
	if decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return nil, ErrInvalidImage
	}
	out := &limitedImageBuffer{}
	normalizedType := "image/png"
	if format == "jpeg" {
		normalizedType = "image/jpeg"
		err = jpeg.Encode(out, decoded, &jpeg.Options{Quality: 90})
	} else {
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		err = encoder.Encode(out, decoded)
	}
	if err != nil {
		return nil, err
	}
	return &NormalizedImage{Data: out.Bytes(), ContentType: normalizedType}, nil
}

type limitedImageBuffer struct{ bytes.Buffer }

func (b *limitedImageBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxStoredImageBytes {
		return 0, ErrImageTooLarge
	}
	return b.Buffer.Write(p)
}

func ImageKey(id, contentType string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return "", ErrInvalidImageKey
	}
	extension := ".png"
	if contentType == "image/jpeg" {
		extension = ".jpg"
	} else if contentType != "image/png" {
		return "", ErrInvalidImageKey
	}
	return "images/" + id + extension, nil
}

func ValidateImageKey(id, key string) error {
	for _, contentType := range []string{"image/png", "image/jpeg"} {
		want, err := ImageKey(id, contentType)
		if err == nil && want == key {
			return nil
		}
	}
	return ErrInvalidImageKey
}

func validateManagedKey(key string) error {
	if !strings.HasPrefix(key, "images/") {
		return ErrInvalidImageKey
	}
	id := strings.TrimSuffix(strings.TrimPrefix(key, "images/"), path.Ext(key))
	if err := ValidateImageKey(id, key); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

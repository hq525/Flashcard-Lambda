package storage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"path"
	"strings"

	"github.com/gen2brain/webp"
	"github.com/google/uuid"
	xwebp "golang.org/x/image/webp"
)

const (
	MaxUploadBytes      = 4 << 20
	MaxLegacyImageBytes = 10 << 20
	MaxStoredImageBytes = 10 << 20
	maxImageDimension   = 8192
	maxImagePixels      = 16_000_000
	// Lossless WebP peaks near 90 MB per megapixel; 8 MP fits the 1024 MB
	// Lambda with headroom. Larger images fall back to PNG.
	maxWebPPixels = 8_000_000
)

var ErrInvalidImage = errors.New("invalid or unsupported image")
var ErrImageTooLarge = errors.New("image exceeds size or dimension limits")
var ErrInvalidImageKey = errors.New("image storage key is invalid or does not belong to this record")

type NormalizedImage struct {
	Data        []byte
	ContentType string
}

type imageDecoder struct {
	config func(io.Reader) (image.Config, error)
	decode func(io.Reader) (image.Image, error)
}

// Decoders are chosen by declared type instead of image.Decode's registry:
// gen2brain/webp registers a libwebp "webp" decoder that would otherwise take
// precedence, and untrusted uploads stay on the pure-Go decoders. A decoder
// rejects bytes of any other format, so the declared type must match.
var decoders = map[string]imageDecoder{
	"image/png":  {png.DecodeConfig, png.Decode},
	"image/jpeg": {jpeg.DecodeConfig, jpeg.Decode},
	"image/gif":  {gif.DecodeConfig, gif.Decode},
	"image/webp": {xwebp.DecodeConfig, xwebp.Decode},
}

func SupportedImageType(contentType string) bool {
	_, ok := decoders[contentType]
	return ok
}

// NormalizeImage fully decodes then re-encodes the pixels, discarding metadata,
// trailing bytes, active content, and animation. JPEG stays JPEG; everything
// else is stored as lossless WebP (pixel-identical), or PNG when too large to
// encode within Lambda memory. HTTP uploads have a smaller bound than the
// administrative migration, which shares this validation.
func NormalizeImage(data []byte, contentType string) (*NormalizedImage, error) {
	decoder, ok := decoders[contentType]
	if len(data) == 0 || !ok {
		return nil, ErrInvalidImage
	}
	if len(data) > MaxLegacyImageBytes {
		return nil, ErrImageTooLarge
	}
	cfg, err := decoder.config(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalidImage
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageDimension || cfg.Height > maxImageDimension || pixels > maxImagePixels {
		return nil, ErrImageTooLarge
	}
	decoded, err := decoder.decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalidImage
	}
	if decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return nil, ErrInvalidImage
	}
	out := &limitedImageBuffer{}
	normalizedType := "image/webp"
	switch {
	case contentType == "image/jpeg":
		normalizedType = "image/jpeg"
		err = jpeg.Encode(out, decoded, &jpeg.Options{Quality: 90})
	case pixels <= maxWebPPixels:
		// Method must be explicit: a zero Method selects the fastest, largest encoding.
		err = webp.Encode(out, toNRGBA(decoded), webp.Options{Lossless: true, Method: 4})
	default:
		normalizedType = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(out, decoded)
	}
	if err != nil {
		return nil, err
	}
	return &NormalizedImage{Data: out.Bytes(), ContentType: normalizedType}, nil
}

// toNRGBA converts with Go's colour models; the encoder would otherwise pass
// premultiplied *image.RGBA pixels to libwebp as straight alpha.
func toNRGBA(m image.Image) *image.NRGBA {
	if n, ok := m.(*image.NRGBA); ok {
		return n
	}
	b := m.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Bounds(), m, b.Min, draw.Src)
	return n
}

type limitedImageBuffer struct{ bytes.Buffer }

func (b *limitedImageBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxStoredImageBytes {
		return 0, ErrImageTooLarge
	}
	return b.Buffer.Write(p)
}

// storedExtensions lists every format a managed key may hold. PNG keys stay
// valid for images stored before WebP and for the oversized-image fallback.
var storedExtensions = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

func storedContentType(key string) string {
	for contentType, extension := range storedExtensions {
		if path.Ext(key) == extension {
			return contentType
		}
	}
	return ""
}

func ImageKey(id, contentType string) (string, error) {
	parsed, err := uuid.Parse(id)
	extension, ok := storedExtensions[contentType]
	if err != nil || parsed.String() != id || !ok {
		return "", ErrInvalidImageKey
	}
	return "images/" + id + extension, nil
}

func ValidateImageKey(id, key string) error {
	if want, err := ImageKey(id, storedContentType(key)); err == nil && want == key {
		return nil
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

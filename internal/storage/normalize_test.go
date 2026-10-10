package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	xwebp "golang.org/x/image/webp"
)

func pngFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	im.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNormalizeImageRejectsDisguisedOrInvalidImages(t *testing.T) {
	for _, tc := range []struct {
		body []byte
		mime string
	}{
		{[]byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><script>alert(1)</script></svg>"), "image/svg+xml"},
		{[]byte("<html>not an image</html>"), "image/png"},
		{pngFixture(t, 2, 2), "image/jpeg"},
		{pngFixture(t, 8193, 1), "image/png"},
		{pngFixture(t, 2, 2)[:20], "image/png"},
	} {
		if _, err := NormalizeImage(tc.body, tc.mime); err == nil {
			t.Fatalf("accepted invalid %s", tc.mime)
		}
	}
}

func TestNormalizeImageReencodesAndStripsTrailingContent(t *testing.T) {
	raw := append(pngFixture(t, 2, 2), []byte("<script>untrusted-trailer</script>")...)
	got, err := NormalizeImage(raw, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentType != "image/webp" || bytes.Contains(got.Data, []byte("untrusted-trailer")) {
		t.Fatal("unvalidated bytes survived normalization")
	}
	im, _, err := image.Decode(bytes.NewReader(got.Data))
	if err != nil || im.Bounds().Dx() != 2 || im.Bounds().Dy() != 2 {
		t.Fatalf("invalid normalized image: %v", err)
	}
}

func TestManagedKeyIsBoundToImageRecord(t *testing.T) {
	id := "e3d4a94b-f0e9-46af-a2c0-2c02850b539a"
	key, err := ImageKey(id, "image/png")
	if err != nil || key != "images/e3d4a94b-f0e9-46af-a2c0-2c02850b539a.png" {
		t.Fatalf("%q %v", key, err)
	}
	webpKey, err := ImageKey(id, "image/webp")
	if err != nil || webpKey != "images/e3d4a94b-f0e9-46af-a2c0-2c02850b539a.webp" || ValidateImageKey(id, webpKey) != nil {
		t.Fatalf("%q %v", webpKey, err)
	}
	for _, bad := range []string{"images/63315953-b5c4-4a0c-9459-787f61368daa.png", "https://bucket.s3.amazonaws.com/" + key, strings.Replace(key, ".png", ".svg", 1)} {
		if ValidateImageKey(id, bad) == nil {
			t.Fatalf("accepted forged key %q", bad)
		}
	}
}

// Screenshot-like pixels: flat background, antialiased greys, a few colours,
// and partial transparency, so a lossy encoder would visibly change values.
func screenshotFixture(width, height int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBA{R: 30, G: 30, B: 30, A: 255}
			if (x/3+y/5)%7 == 0 {
				c = color.NRGBA{R: uint8(x * 13), G: uint8(y * 7), B: 214, A: 255}
			}
			if y%11 == 0 {
				c.A = uint8(64 + x%128)
			}
			im.Set(x, y, c)
		}
	}
	return im
}

func encodePNG(t *testing.T, im image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNormalizeImageStoresNonJPEGAsLosslessWebP(t *testing.T) {
	src := screenshotFixture(97, 61)
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, src, nil); err != nil {
		t.Fatal(err)
	}
	gifSrc, err := gif.Decode(bytes.NewReader(gifBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mime string
		body       []byte
		want       image.Image
	}{
		{"png", "image/png", encodePNG(t, src), src},
		{"gif", "image/gif", gifBuf.Bytes(), gifSrc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeImage(tc.body, tc.mime)
			if err != nil {
				t.Fatal(err)
			}
			if got.ContentType != "image/webp" {
				t.Fatalf("content type %q; want image/webp", got.ContentType)
			}
			stored, err := xwebp.Decode(bytes.NewReader(got.Data))
			if err != nil {
				t.Fatal(err)
			}
			assertSamePixels(t, tc.want, stored)
		})
	}
}

func assertSamePixels(t *testing.T, want, got image.Image) {
	t.Helper()
	if want.Bounds() != got.Bounds() {
		t.Fatalf("bounds %v; want %v", got.Bounds(), want.Bounds())
	}
	for y := want.Bounds().Min.Y; y < want.Bounds().Max.Y; y++ {
		for x := want.Bounds().Min.X; x < want.Bounds().Max.X; x++ {
			w := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
			g := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			if w != g {
				t.Fatalf("pixel (%d,%d) = %v; want %v", x, y, g, w)
			}
		}
	}
}

func TestNormalizeImageKeepsJPEGAsJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, screenshotFixture(40, 30), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeImage(buf.Bytes(), "image/jpeg")
	if err != nil || got.ContentType != "image/jpeg" {
		t.Fatalf("got %v %v; want image/jpeg", got, err)
	}
}

func TestNormalizeImageFallsBackToPNGAboveWebPMemoryBudget(t *testing.T) {
	width, height := 4000, maxWebPPixels/4000+1
	got, err := NormalizeImage(encodePNG(t, image.NewGray(image.Rect(0, 0, width, height))), "image/png")
	if err != nil || got.ContentType != "image/png" {
		t.Fatalf("got %v; want image/png fallback", err)
	}
}

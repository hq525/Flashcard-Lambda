package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
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
	if got.ContentType != "image/png" || bytes.Contains(got.Data, []byte("untrusted-trailer")) {
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
	for _, bad := range []string{"images/63315953-b5c4-4a0c-9459-787f61368daa.png", "https://bucket.s3.amazonaws.com/" + key, strings.Replace(key, ".png", ".svg", 1)} {
		if ValidateImageKey(id, bad) == nil {
			t.Fatalf("accepted forged key %q", bad)
		}
	}
}

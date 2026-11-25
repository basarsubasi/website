package photos

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"testing"

	"github.com/gen2brain/vpx/webp"

	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	// JPEG APP1 with a little-endian TIFF orientation tag of 6 (90 degrees clockwise).
	rotateClockwiseEXIF    = "\xff\xe1\x00\x22Exif\x00\x00II\x2a\x00\x08\x00\x00\x00\x01\x00\x12\x01\x03\x00\x01\x00\x00\x00\x06\x00\x00\x00\x00\x00\x00\x00"
	originalMismatch       = "preview generation changed the original or left it partially consumed"
	dimensionsMismatch     = "unexpected oriented preview dimensions: %v"
	oversizedInputAccepted = "oversized gallery image accepted"
)

func TestPreviewAppliesJPEGOrientationAndRewindsOriginal(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewGray(image.Rect(0, 0, 1200, 600)), nil); err != nil {
		t.Fatal(err)
	}
	original := append([]byte{0xff, 0xd8}, rotateClockwiseEXIF...)
	original = append(original, jpegData.Bytes()[2:]...)
	source := bytes.NewReader(original)
	info, err := media.Inspect(source, int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := RenderPreview(t.Context(), source, info)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if bounds := preview.Bounds(); bounds.Dx() != 480 || bounds.Dy() != 960 {
		t.Fatalf(dimensionsMismatch, bounds)
	}
	raw, err = io.ReadAll(source)
	if err != nil || !bytes.Equal(raw, original) {
		t.Fatal(originalMismatch, err)
	}
}

func TestPreviewChecksDimensionsBeforeDecoding(t *testing.T) {
	for _, info := range []media.FileInfo{{Width: 6001, Height: 4000}, {Width: previewDimensionLimit + 1, Height: 1}} {
		if _, err := RenderPreview(t.Context(), nil, info); err == nil {
			t.Fatal(oversizedInputAccepted)
		}
	}
}

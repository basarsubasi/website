package photos

import (
	"bytes"
	"context"
	"io"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/vpx/webp"

	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	previewSize            = 960
	previewQuality         = 85
	previewContentType     = "image/webp"
	previewPixelLimit      = 24_000_000
	previewDimensionLimit  = 16_384
	previewTooLargeMessage = "Gallery photos must be at most 24 megapixels and 16384 pixels per side"
	invalidPreviewSource   = "Could not decode gallery photo"
)

// Decode and resize only one full-resolution image at a time.
var previewSlot = make(chan struct{}, 1)

// RenderPreview reads a validated upload and rewinds it for original-byte storage.
func RenderPreview(ctx context.Context, file io.ReadSeeker, info media.FileInfo) ([]byte, error) {
	if info.Width <= 0 || info.Height <= 0 || max(info.Width, info.Height) > previewDimensionLimit || info.Width > previewPixelLimit/info.Height {
		return nil, domain.Invalid(previewTooLargeMessage)
	}
	select {
	case previewSlot <- struct{}{}:
		defer func() { <-previewSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	src, err := imaging.Decode(file, imaging.AutoOrientation(true))
	if err != nil {
		return nil, domain.Invalid(invalidPreviewSource)
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	preview := imaging.Fit(src, previewSize, previewSize, imaging.Lanczos)
	var encoded bytes.Buffer
	if err := webp.Encode(&encoded, preview, webp.EncodeOptions{Quality: previewQuality, Method: 5, Threads: 1}); err != nil {
		return nil, err
	}
	return encoded.Bytes(), ctx.Err()
}

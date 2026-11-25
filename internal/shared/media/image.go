package media

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	invalidImageMessage     = "Upload a valid JPEG, PNG, or GIF image"
	jpegFormat              = "jpeg"
	jpegContentType         = "image/jpeg"
	pngFormat               = "png"
	pngContentType          = "image/png"
	gifFormat               = "gif"
	gifContentType          = "image/gif"
	unsupportedImageMessage = "Upload a JPEG, PNG, or GIF image"
	jpegExtension           = "jpg"
)

const MaxSize = 25 << 20

type FileInfo struct {
	Width, Height          int
	ContentType, Extension string
}

func Inspect(file io.ReadSeeker, size int64) (FileInfo, error) {
	if size < 1 || size > MaxSize {
		return FileInfo{}, domain.ErrTooLarge
	}
	dimensions, format, err := image.DecodeConfig(file)
	if err != nil {
		return FileInfo{}, domain.Invalid(invalidImageMessage)
	}
	contentType, ok := map[string]string{jpegFormat: jpegContentType, pngFormat: pngContentType, gifFormat: gifContentType}[format]
	if !ok {
		return FileInfo{}, domain.Invalid(unsupportedImageMessage)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return FileInfo{}, err
	}
	ext := format
	if ext == jpegFormat {
		ext = jpegExtension
	}
	return FileInfo{Width: dimensions.Width, Height: dimensions.Height, ContentType: contentType, Extension: ext}, nil
}

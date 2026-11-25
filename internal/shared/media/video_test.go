package media

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	clipFixture           = "../../../tests/testdata/blog-clip.mp4"
	videoInfoMismatch     = "unexpected clip metadata: %+v"
	videoRewindFailure    = "upload stream was not rewound"
	unexpectedVideoResult = "expected video validation error, got %v"
	wrongVideoCodec       = "vp09"
	wrongAudioCodec       = "alac"
)

func TestInspectVideo(t *testing.T) {
	raw, err := os.ReadFile(clipFixture)
	if err != nil {
		t.Fatal(err)
	}
	file := bytes.NewReader(raw)
	info, err := InspectVideo(file, int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 64 || info.Height != 48 || info.ContentType != VideoContentType || info.Extension != videoExtension {
		t.Fatalf(videoInfoMismatch, info)
	}
	if pos, err := file.Seek(0, io.SeekCurrent); err != nil || pos != 0 {
		t.Fatal(videoRewindFailure, err)
	}
	for _, sample := range [][]byte{
		nil, raw[:4], raw[:len(raw)-1],
		bytes.ReplaceAll(raw, []byte(avc1Entry), []byte(wrongVideoCodec)),
		bytes.ReplaceAll(raw, []byte(aacEntry), []byte(wrongAudioCodec)),
	} {
		_, err = InspectVideo(bytes.NewReader(sample), int64(len(sample)))
		var invalid *domain.ValidationError
		if !errors.As(err, &invalid) {
			t.Fatalf(unexpectedVideoResult, err)
		}
	}
	if _, err = InspectVideo(file, MaxSize+1); !errors.Is(err, domain.ErrVideoTooLarge) {
		t.Fatalf(unexpectedVideoResult, err)
	}
}

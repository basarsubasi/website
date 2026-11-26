package media

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/basarsubasi/website/internal/shared/domain/validation"
)

const (
	av1OpusFixture        = "../../../tests/testdata/blog-clip-av1-opus.mp4"
	silentAV1Fixture      = "../../../tests/testdata/blog-clip-av1.mp4"
	av1AACFixture         = "../../../tests/testdata/blog-clip-av1-aac.mp4"
	h264OpusFixture       = "../../../tests/testdata/blog-clip-opus.mp4"
	h264AACFixture        = "../../../tests/testdata/blog-clip.mp4"
	videoInfoMismatch     = "unexpected clip metadata: %+v"
	videoRewindFailure    = "upload stream was not rewound"
	unexpectedVideoResult = "expected video validation error, got %v"
	av1ConfigBox          = "av1C"
	opusConfigBox         = "dOps"
	freeSpaceBox          = "free"
	opusConfigMismatch    = "Opus config %+v: got %t, want %t"
)

func TestInspectVideo(t *testing.T) {
	for _, fixture := range []string{av1OpusFixture, silentAV1Fixture} {
		t.Run(fixture, func(t *testing.T) {
			file := bytes.NewReader(readVideoFixture(t, fixture))
			info, err := InspectVideo(file, file.Size())
			if err != nil {
				t.Fatal(err)
			}
			if info.Width != 64 || info.Height != 48 || info.ContentType != VideoContentType || info.Extension != videoExtension {
				t.Fatalf(videoInfoMismatch, info)
			}
			if pos, err := file.Seek(0, io.SeekCurrent); err != nil || pos != 0 {
				t.Fatal(videoRewindFailure, err)
			}
		})
	}
}

func TestInspectVideoRejectsInvalidFiles(t *testing.T) {
	raw := readVideoFixture(t, av1OpusFixture)
	for _, sample := range [][]byte{
		nil, raw[:4], raw[:len(raw)-1],
		readVideoFixture(t, h264AACFixture), readVideoFixture(t, h264OpusFixture), readVideoFixture(t, av1AACFixture),
		bytes.ReplaceAll(raw, []byte(av1ConfigBox), []byte(freeSpaceBox)),
		bytes.ReplaceAll(raw, []byte(opusConfigBox), []byte(freeSpaceBox)),
	} {
		_, err := InspectVideo(bytes.NewReader(sample), int64(len(sample)))
		var invalid *validation.Error
		if !errors.As(err, &invalid) {
			t.Fatalf(unexpectedVideoResult, err)
		}
	}
	if _, err := InspectVideo(bytes.NewReader(raw), MaxVideoSize+1); !errors.Is(err, ErrVideoTooLarge) {
		t.Fatalf(unexpectedVideoResult, err)
	}
}

func readVideoFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOpusDecoderConfig(t *testing.T) {
	for _, tc := range []struct {
		config mp4.DopsBox
		valid  bool
	}{
		{mp4.DopsBox{OutputChannelCount: 2}, true},
		{mp4.DopsBox{OutputChannelCount: 6, ChannelMappingFamily: 1, StreamCount: 4, CoupledCount: 2, ChannelMapping: []byte{0, 4, 1, 2, 3, 5}}, true},
		{mp4.DopsBox{Version: 1, OutputChannelCount: 2}, false},
		{mp4.DopsBox{}, false},
		{mp4.DopsBox{OutputChannelCount: 3}, false},
		{mp4.DopsBox{OutputChannelCount: 2, ChannelMappingFamily: 1, StreamCount: 1, ChannelMapping: []byte{0, 1}}, false},
	} {
		if got := validOpusConfig(&tc.config); got != tc.valid {
			t.Fatalf(opusConfigMismatch, tc.config, got, tc.valid)
		}
	}
}

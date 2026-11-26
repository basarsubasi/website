package media

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/basarsubasi/website/internal/shared/domain/validation"
)

const (
	opusFixture = "../../../tests/testdata/blog-clip.opus"
	// A checksummed Ogg Vorbis identification page from libvorbis.
	vorbisID              = "4f67675300020000000000000000e1307ad00000000025d39d5c011e01766f72626973000000000144ac0000000000008038010000000000b801"
	audioMetadataMismatch = "unexpected audio metadata: %+v"
	unexpectedAudioResult = "expected audio validation error for %d bytes, got %v"
)

func TestInspectAudio(t *testing.T) {
	raw, err := os.ReadFile(opusFixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{raw, bytes.Repeat(raw, 2)} {
		file := bytes.NewReader(data)
		info, err := InspectAudio(file, file.Size())
		if err != nil {
			t.Fatal(err)
		}
		if info != (FileInfo{ContentType: AudioContentType, Extension: audioExtension}) {
			t.Fatalf(audioMetadataMismatch, info)
		}
		if pos, err := file.Seek(0, io.SeekCurrent); pos != 0 || err != nil {
			t.Fatal(videoRewindFailure, err)
		}
	}
	vorbis, err := hex.DecodeString(vorbisID)
	if err != nil {
		t.Fatal(err)
	}
	badCRC := bytes.Clone(raw)
	badCRC[len(badCRC)-1] ^= 1
	lastPage := bytes.LastIndex(raw, []byte(oggCapture))
	for _, data := range [][]byte{
		nil, vorbis, badCRC, raw[:len(raw)-1], raw[:lastPage], raw[:lastPage+oggHeaderSize],
		bytes.Join([][]byte{raw, vorbis}, nil),
	} {
		_, err := InspectAudio(bytes.NewReader(data), int64(len(data)))
		var invalid *validation.Error
		if !errors.As(err, &invalid) {
			t.Fatalf(unexpectedAudioResult, len(data), err)
		}
	}
	if _, err := InspectAudio(bytes.NewReader(raw), MaxSize+1); !errors.Is(err, ErrAudioTooLarge) {
		t.Fatalf(unexpectedAudioResult, MaxSize+1, err)
	}
}

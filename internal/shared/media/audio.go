package media

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/pion/opus/pkg/oggreader"

	"github.com/basarsubasi/website/internal/shared/domain/validation"
)

const (
	audioTooLargeMessage = "Audio must be at most 25 MiB"
	AudioContentType     = "audio/ogg"
	audioExtension       = "opus"
	invalidAudio         = "Upload an Ogg Opus audio file"
	opusTags             = "OpusTags"
	oggCapture           = "OggS"
	oggHeaderSize        = 27
	oggBOS               = 2
	oggEOS               = 4
)

var ErrAudioTooLarge = errors.New(audioTooLargeMessage)

// InspectAudio validates every logical stream, including chained Ogg files,
// then rewinds the upload so storage receives the original bytes.
func InspectAudio(file io.ReadSeeker, size int64) (FileInfo, error) {
	if size > MaxSize {
		return FileInfo{}, ErrAudioTooLarge
	}
	source := &io.LimitedReader{R: file, N: size}
	input := bufio.NewReader(source)
	for {
		if !validOpusStream(&oggStream{input: input}) {
			return FileInfo{}, validation.Invalid(invalidAudio)
		}
		if source.N == 0 && input.Buffered() == 0 {
			break
		}
	}
	_, err := file.Seek(0, io.SeekStart)
	return FileInfo{ContentType: AudioContentType, Extension: audioExtension}, err
}

func validOpusStream(stream *oggStream) bool {
	reader, header, err := oggreader.NewWith(stream)
	if err != nil || header.Version != 1 || header.Channels == 0 || (header.ChannelMap == 0 && header.Channels > 2) {
		return false
	}
	tags, _, err := reader.ParseNextPacket()
	if err != nil || !validOpusTags(tags) {
		return false
	}
	hasAudio := false
	for {
		packet, _, err := reader.ParseNextPacket()
		if err != nil {
			return err == io.EOF && hasAudio
		}
		if len(packet) == 0 {
			return false
		}
		hasAudio = true
	}
}

// The mandatory comment header contains a vendor string, a count, and that
// many length-prefixed comments. Trailing padding is allowed.
func validOpusTags(packet []byte) bool {
	data, ok := bytes.CutPrefix(packet, []byte(opusTags))
	if !ok {
		return false
	}
	skipString := func() bool {
		if len(data) < 4 {
			return false
		}
		size := binary.LittleEndian.Uint32(data)
		data = data[4:]
		if uint64(size) > uint64(len(data)) {
			return false
		}
		data = data[size:]
		return true
	}
	if !skipString() || len(data) < 4 {
		return false
	}
	count := binary.LittleEndian.Uint32(data)
	data = data[4:]
	for range count {
		if !skipString() {
			return false
		}
	}
	return true
}

// Pion reassembles packets and checks page CRCs, but does not expose stream
// boundaries. Present one complete serial at a time so a second codec or a
// truncated last page cannot be mistaken for part of the first Opus stream.
type oggStream struct {
	input     *bufio.Reader
	remaining int
	serial    uint32
	nextPage  uint32
	last      bool
}

func (s *oggStream) Read(dst []byte) (int, error) {
	if s.remaining == 0 {
		if s.last {
			return 0, io.EOF
		}
		if err := s.startPage(); err != nil {
			return 0, err
		}
	}
	n, err := s.input.Read(dst[:min(len(dst), s.remaining)])
	s.remaining -= n
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func (s *oggStream) startPage() error {
	header, err := s.input.Peek(oggHeaderSize)
	if err != nil || string(header[:4]) != oggCapture || header[4] != 0 {
		return io.ErrUnexpectedEOF
	}
	flags := header[5]
	serial := binary.LittleEndian.Uint32(header[14:])
	sequence := binary.LittleEndian.Uint32(header[18:])
	if sequence != s.nextPage || (flags&oggBOS != 0) != (sequence == 0) || (sequence != 0 && serial != s.serial) {
		return io.ErrUnexpectedEOF
	}
	tableEnd := oggHeaderSize + int(header[26])
	page, err := s.input.Peek(tableEnd)
	if err != nil {
		return io.ErrUnexpectedEOF
	}
	s.remaining = tableEnd
	for _, length := range page[oggHeaderSize:] {
		s.remaining += int(length)
	}
	s.serial = serial
	s.nextPage++
	s.last = flags&oggEOS != 0
	return nil
}

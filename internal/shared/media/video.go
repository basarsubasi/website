package media

import (
	"bytes"
	"io"
	"slices"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	VideoContentType = "video/mp4"
	videoExtension   = "mp4"
	invalidVideo     = "Upload an MP4 with H.264 video and optional AAC audio"
	fileTypeBox      = "ftyp"
	movieBox         = "moov"
	mediaDataBox     = "mdat"
	videoHandler     = "vide"
	audioHandler     = "soun"
	avc1Entry        = "avc1"
	avc3Entry        = "avc3"
	aacEntry         = "mp4a"
	// Bound metadata parsing independently of the encoded video payload.
	maxMovieHeader    = 2 << 20
	maxFileTypeHeader = 4096
	maxTopLevelBoxes  = 8192
)

// InspectVideo reads track metadata and skips the encoded frames. The original
// file is rewound for streaming to storage, including when moov follows mdat.
func InspectVideo(file io.ReadSeeker, size int64) (FileInfo, error) {
	if size > MaxSize {
		return FileInfo{}, domain.ErrVideoTooLarge
	}
	var movie videoMetadata
	for offset, count := int64(0), 0; offset < size; count++ {
		hdr, err := mp4.DecodeHeader(file)
		if err != nil || hdr.Size > uint64(size-offset) || count >= maxTopLevelBoxes {
			return FileInfo{}, domain.Invalid(invalidVideo)
		}
		if err := movie.readBox(hdr, uint64(offset), file); err != nil {
			return FileInfo{}, err
		}
		offset += int64(hdr.Size)
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return FileInfo{}, err
		}
	}
	if !movie.hasType || !movie.hasData || movie.info.Width == 0 {
		return FileInfo{}, domain.Invalid(invalidVideo)
	}
	_, err := file.Seek(0, io.SeekStart)
	return movie.info, err
}

type videoMetadata struct {
	info             FileInfo
	hasType, hasData bool
}

func (m *videoMetadata) readBox(hdr mp4.BoxHeader, offset uint64, file io.Reader) error {
	switch hdr.Name {
	case fileTypeBox:
		if m.hasType || !isMP4FileType(hdr, file) {
			return domain.Invalid(invalidVideo)
		}
		m.hasType = true
	case movieBox:
		if m.info.Width != 0 || hdr.Size > maxMovieHeader {
			return domain.Invalid(invalidVideo)
		}
		box, err := mp4.DecodeMoov(hdr, offset, file)
		if err != nil {
			return domain.Invalid(invalidVideo)
		}
		for _, track := range box.(*mp4.MoovBox).Traks {
			if !m.acceptTrack(track) {
				return domain.Invalid(invalidVideo)
			}
		}
		if m.info.Width == 0 {
			return domain.Invalid(invalidVideo)
		}
	case mediaDataBox:
		m.hasData = m.hasData || hdr.Size > uint64(hdr.Hdrlen)
	}
	return nil
}

func isMP4FileType(hdr mp4.BoxHeader, file io.Reader) bool {
	if hdr.Size > maxFileTypeHeader {
		return false
	}
	box, err := mp4.DecodeFtyp(hdr, 0, file)
	if err != nil {
		return false
	}
	ftyp := box.(*mp4.FtypBox)
	brands := []string{mp4.BrandIsom, mp4.BrandMp41, mp4.BrandMp42, mp4.BrandIso6, avc1Entry}
	return slices.Contains(brands, ftyp.MajorBrand()) || slices.ContainsFunc(brands, ftyp.HasCompatibleBrand)
}

func (m *videoMetadata) acceptTrack(track *mp4.TrakBox) bool {
	mdia := track.Mdia
	if mdia == nil || mdia.Hdlr == nil || track.CheckDataIsSelfContained() != nil {
		return false
	}
	kind := mdia.Hdlr.HandlerType
	if kind != videoHandler && kind != audioHandler {
		return true
	}
	if mdia.Minf == nil || mdia.Minf.Stbl == nil || mdia.Minf.Stbl.Stsd == nil {
		return false
	}
	entries := mdia.Minf.Stbl.Stsd.Children
	if len(entries) == 0 {
		return false
	}
	for _, entry := range entries {
		if !m.acceptSampleEntry(kind, entry) {
			return false
		}
	}
	return true
}

func (m *videoMetadata) acceptSampleEntry(kind string, entry mp4.Box) bool {
	if kind == audioHandler {
		return isAAC(entry)
	}
	video, ok := entry.(*mp4.VisualSampleEntryBox)
	if !ok || (video.Type() != avc1Entry && video.Type() != avc3Entry) || video.AvcC == nil || video.Width == 0 || video.Height == 0 {
		return false
	}
	if m.info.Width == 0 {
		m.info = FileInfo{ContentType: VideoContentType, Extension: videoExtension, Width: int(video.Width), Height: int(video.Height)}
	}
	return true
}

func isAAC(entry mp4.Box) bool {
	audio, ok := entry.(*mp4.AudioSampleEntryBox)
	if !ok || audio.Type() != aacEntry || audio.Esds == nil || audio.Esds.DecConfigDescriptor == nil {
		return false
	}
	config := audio.Esds.DecConfigDescriptor
	if config.ObjectType != mp4.ObjectTypeAAC || config.DecSpecificInfo == nil {
		return false
	}
	_, err := aac.DecodeAudioSpecificConfig(bytes.NewReader(config.DecSpecificInfo.DecConfig))
	return err == nil
}

package article

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

const (
	AudioKeyPrefix       = "audio/"
	VideoKeyPrefix       = "videos/"
	uuidFilenamePrefix   = `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.`
	imageFilenamePattern = uuidFilenamePrefix + `(jpg|png|gif)$`
	audioFilenamePattern = uuidFilenamePrefix + `opus$`
	videoFilenamePattern = uuidFilenamePrefix + `mp4$`
)

var (
	imageFilename = regexp.MustCompile(imageFilenamePattern)
	audioFilename = regexp.MustCompile(audioFilenamePattern)
	videoFilename = regexp.MustCompile(videoFilenamePattern)
)

func ValidImageFilename(name string) bool { return imageFilename.MatchString(name) }
func ValidVideoFilename(name string) bool { return videoFilename.MatchString(name) }
func ValidAudioFilename(name string) bool { return audioFilename.MatchString(name) }

func isMediaObjectURL(raw, prefix string, validFilename func(string) bool) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	dir, name := path.Split(u.EscapedPath())
	return strings.HasSuffix(dir, "/"+prefix) && validFilename(name)
}

// BlogAsset describes an uploaded original without changing its bytes.
type BlogAsset struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

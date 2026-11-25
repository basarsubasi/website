package article

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

const (
	VideoKeyPrefix       = "videos/"
	uuidFilenamePrefix   = `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.`
	imageFilenamePattern = uuidFilenamePrefix + `(jpg|png|gif)$`
	videoFilenamePattern = uuidFilenamePrefix + `mp4$`
)

var (
	imageFilename = regexp.MustCompile(imageFilenamePattern)
	videoFilename = regexp.MustCompile(videoFilenamePattern)
)

func ValidImageFilename(name string) bool { return imageFilename.MatchString(name) }
func ValidVideoFilename(name string) bool { return videoFilename.MatchString(name) }

func isVideoObjectURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	dir, name := path.Split(u.EscapedPath())
	return strings.HasSuffix(dir, "/"+VideoKeyPrefix) && ValidVideoFilename(name)
}

// BlogAsset describes an uploaded original without changing its bytes.
type BlogAsset struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

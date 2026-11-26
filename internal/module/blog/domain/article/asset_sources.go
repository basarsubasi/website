package article

import (
	"encoding/json"
	"strings"

	"github.com/basarsubasi/website/internal/shared/domain/validation"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	documentPrefix         = "blog-content-v1:"
	nodeTypeKey            = "type"
	audioNodeType          = "blogAudio"
	untrustedAudioMessage  = "Use audio uploaded to the configured blog asset origin"
	videoNodeType          = "blogVideo"
	untrustedVideoMessage  = "Use videos uploaded to the configured blog asset origin"
	imageNodeType          = "blogImage"
	attrsKey               = "attrs"
	sourceKey              = "src"
	invalidDocumentMessage = "Invalid rich-text post body"
	untrustedImageMessage  = "Use images uploaded to the configured blog asset origin"
)

var assetSourceRules = map[string]struct {
	message, prefix string
	validFilename   func(string) bool
}{
	imageNodeType: {message: untrustedImageMessage},
	videoNodeType: {untrustedVideoMessage, VideoKeyPrefix, ValidVideoFilename},
	audioNodeType: {untrustedAudioMessage, AudioKeyPrefix, ValidAudioFilename},
}

// Plain-text posts contain no media embeds. Structured posts use the same
// top-level nodes and case-sensitive JSON keys as the Vue article renderer.
func ValidateAssetSources(body string, allowed media.Origin) error {
	document, structured := strings.CutPrefix(body, documentPrefix)
	if !structured {
		return nil
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(document), &blocks); err != nil || blocks == nil {
		return validation.Invalid(invalidDocumentMessage)
	}
	for _, block := range blocks {
		kind, _ := block[nodeTypeKey].(string)
		rule, ok := assetSourceRules[kind]
		if !ok {
			continue
		}
		attrs, _ := block[attrsKey].(map[string]any)
		src, _ := attrs[sourceKey].(string)
		origin, err := media.ParseOrigin(src)
		if err != nil || origin != allowed {
			return validation.Invalid(rule.message)
		}
		if rule.validFilename != nil && !isMediaObjectURL(src, rule.prefix, rule.validFilename) {
			return validation.Invalid(rule.message)
		}
	}
	return nil
}

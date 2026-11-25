package article

import (
	"encoding/json"
	"strings"

	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	documentPrefix         = "blog-content-v1:"
	nodeTypeKey            = "type"
	imageNodeType          = "blogImage"
	attrsKey               = "attrs"
	sourceKey              = "src"
	invalidDocumentMessage = "Invalid rich-text post body"
	untrustedImageMessage  = "Use images uploaded to the configured blog asset origin"
)

// Plain-text posts contain no image embeds. Structured posts use the same
// top-level nodes and case-sensitive JSON keys as the Vue article renderer.
func ValidateImageSources(body string, allowed media.Origin) error {
	document, structured := strings.CutPrefix(body, documentPrefix)
	if !structured {
		return nil
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(document), &blocks); err != nil || blocks == nil {
		return domain.Invalid(invalidDocumentMessage)
	}
	for _, block := range blocks {
		if block[nodeTypeKey] != imageNodeType {
			continue
		}
		attrs, _ := block[attrsKey].(map[string]any)
		src, _ := attrs[sourceKey].(string)
		origin, err := media.ParseOrigin(src)
		if err != nil || origin != allowed {
			return domain.Invalid(untrustedImageMessage)
		}
	}
	return nil
}

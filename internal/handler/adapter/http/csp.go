package httpadapter

import (
	"strings"

	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	contentSecurityPolicyHeader = "Content-Security-Policy"
	// Tiptap uses inline styles. Scripts load only from the application origin.
	// The site icon is local and the upload preview uses an object URL.
	basePolicy      = "default-src 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self'; img-src 'self' blob:"
	sourceSeparator = " "
)

func contentSecurityPolicy(photos, blog media.Origin) string {
	sources := []string{basePolicy}
	if photos != (media.Origin{}) {
		sources = append(sources, photos.String())
	}
	if blog != (media.Origin{}) && blog != photos {
		sources = append(sources, blog.String())
	}
	return strings.Join(sources, sourceSeparator)
}

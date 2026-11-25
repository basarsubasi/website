package httpadapter

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/media"
)

const (
	imagePostSlug       = "image-policy"
	imagePostTitle      = "Image policy"
	imagePostFormat     = `blog-content-v1:[{"type":"blogImage","attrs":{"src":%q}}]`
	trustedImage        = "https://IMAGES.example.com:443/images/photo.jpg"
	malformedBody       = "blog-content-v1:["
	otherImage          = "https://other.example.com/photo.jpg"
	relativeImage       = "/images/photo.jpg"
	otherPort           = "https://images.example.com:8443/photo.jpg"
	otherScheme         = "http://images.example.com/photo.jpg"
	subdomainImage      = "https://other.images.example.com/photo.jpg"
	credentialedImage   = "https://user@images.example.com/photo.jpg"
	wrongCaseBody       = `blog-content-v1:[{"type":"blogImage","attrs":{"src":"https://other.example.com/image.png","SRC":"https://images.example.com/image.png"}}]`
	plainURL            = "A plain text URL: https://other.example.com/photo.jpg"
	publicImagePostPath = "/api/posts/image-policy"
	postUpdatePrefix    = "/api/admin/posts/"
	saveChangedBody     = "rejected edit changed the saved post"
	photoBase           = "https://photos.example.com/bucket"
	blogBase            = "https://blog.example.com/assets"
	wildcardBase        = "https://*.example.com"
	wildcardAccepted    = "accepted a wildcard as an image origin"
	expectedImages      = "img-src 'self' blob: https://photos.example.com https://blog.example.com"
	unexpectedPolicy    = "unexpected content security policy: %s"
)

func TestSavePostChecksImageOriginBeforeWriting(t *testing.T) {
	c := setup(t)
	c.login()
	post := article.PostInput{Title: imagePostTitle, Slug: imagePostSlug, Published: 1}
	for _, source := range []string{otherImage, relativeImage, otherPort, otherScheme, subdomainImage, credentialedImage} {
		post.Body = fmt.Sprintf(imagePostFormat, source)
		c.request(http.MethodPost, serverTestAdminPostsPath, post, http.StatusBadRequest)
	}
	for _, body := range []string{malformedBody, wrongCaseBody} {
		post.Body = body
		c.request(http.MethodPost, serverTestAdminPostsPath, post, http.StatusBadRequest)
	}
	post.Body = fmt.Sprintf(imagePostFormat, trustedImage)
	saved := item[article.Post](t, c.request(http.MethodPost, serverTestAdminPostsPath, post, http.StatusCreated))
	post.Body = fmt.Sprintf(imagePostFormat, otherImage)
	c.request(http.MethodPut, postUpdatePrefix+saved.ID, post, http.StatusBadRequest)
	got := item[article.Post](t, c.request(http.MethodGet, publicImagePostPath, nil, http.StatusOK))
	if got.Body != saved.Body {
		t.Fatal(saveChangedBody)
	}
	post.Body = plainURL
	c.request(http.MethodPut, postUpdatePrefix+saved.ID, post, http.StatusOK)
}

func TestCSPUsesOnlyConfiguredImageOrigins(t *testing.T) {
	photos, err := media.ParseOrigin(photoBase)
	if err != nil {
		t.Fatal(err)
	}
	blog, err := media.ParseOrigin(blogBase)
	if err != nil {
		t.Fatal(err)
	}
	if policy := contentSecurityPolicy(photos, blog); !strings.HasSuffix(policy, expectedImages) {
		t.Fatalf(unexpectedPolicy, policy)
	}
	if _, err := media.ParseOrigin(wildcardBase); err == nil {
		t.Fatal(wildcardAccepted)
	}
}

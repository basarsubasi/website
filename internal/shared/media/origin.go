package media

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	httpScheme           = "http"
	httpsScheme          = "https"
	schemeSeparator      = "://"
	portSeparator        = ":"
	emptyPort            = ""
	authorityPattern     = `^([a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)*\.?|\[[a-fA-F0-9:.]+\])(:[0-9]+)?$`
	invalidOriginMessage = "expected an absolute HTTP(S) asset URL with a literal host and no credentials"
)

var (
	authority        = regexp.MustCompile(authorityPattern)
	errInvalidOrigin = errors.New(invalidOriginMessage)
)

// Origin is a scheme, host, and effective port suitable for exact comparisons
// and for a CSP host source. Paths and URL parameters never enter the policy.
type Origin struct{ value string }

func ParseOrigin(raw string) (Origin, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != httpScheme && u.Scheme != httpsScheme) || u.User != nil || !authority.MatchString(u.Host) {
		return Origin{}, errInvalidOrigin
	}
	host := strings.ToLower(u.Host)
	if port := u.Port(); port != emptyPort {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return Origin{}, errInvalidOrigin
		}
		host = strings.TrimSuffix(host, portSeparator+port)
		if !(u.Scheme == httpsScheme && number == 443 || u.Scheme == httpScheme && number == 80) {
			host += portSeparator + strconv.FormatUint(number, 10)
		}
	}
	return Origin{u.Scheme + schemeSeparator + host}, nil
}

func (o Origin) String() string { return o.value }

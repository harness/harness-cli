package secret

import (
	"fmt"
	"net/http"
	"strings"
)

// HeaderPrefix is the prefix of the value of Authorization header.
// It has the space.
const HeaderPrefix = "Harbor-Secret "

// FromRequest tries to get Harbor Secret from request header.
// It will return empty string if the request is nil.
func FromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	auth := req.Header.Get("Authorization")
	if strings.HasPrefix(auth, HeaderPrefix) {
		return strings.TrimPrefix(auth, HeaderPrefix)
	}
	return ""
}

// AddToRequest add the secret to request
func AddToRequest(req *http.Request, secret string) error {
	if req == nil {
		return fmt.Errorf("input request is nil, unable to set secret")
	}
	req.Header.Set("Authorization", fmt.Sprintf("%s%s", HeaderPrefix, secret))
	return nil
}

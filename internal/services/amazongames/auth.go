package amazongames

import (
	"errors"
	"net/url"
	"strings"
)

// ParseAuthCode extracts the authorization code from what the user pasted
// after signing in: the full address Amazon landed on (its
// openid.oa2.authorization_code parameter), or the bare code.
func ParseAuthCode(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", errors.New("paste the address Amazon took you to after signing in")
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		// Not a URL: the whole input is the code.
		return trimmed, nil
	}
	code := u.Query().Get("openid.oa2.authorization_code")
	if code == "" {
		return "", errors.New("that address has no sign-in code: copy the full address from the browser's address bar after signing in to Amazon")
	}
	return code, nil
}

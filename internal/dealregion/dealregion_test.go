package dealregion

import "testing"

func TestValid(t *testing.T) {
	for _, code := range []string{"us", "gb", "jp", "br"} {
		if !Valid(code) {
			t.Errorf("Valid(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"", "US", "zz", "usa", "u"} {
		if Valid(code) {
			t.Errorf("Valid(%q) = true, want false", code)
		}
	}
}

package contract

import "testing"

func TestValidateHTTPSURL(t *testing.T) {
	accept := []string{
		"https://control.example.com",
		"https://control.example.com:8081",
		"https://control.example.com:8081/path",
		"  https://control.example.com  ",
		"Https://control.example.com",
	}
	for _, u := range accept {
		t.Run("ok/"+u, func(t *testing.T) {
			if err := ValidateHTTPSURL(u); err != nil {
				t.Errorf("ValidateHTTPSURL(%q) = %v, want nil", u, err)
			}
		})
	}

	reject := []string{
		"",
		"http://control.example.com",
		"HTTP://control.example.com",
		"h2c://control.example.com",
		"ftp://x",
		"control.example.com",
		"https:foo",
		"https:",
		"https://",
		"https://user:pass@host",
		"https://host#frag",
	}
	for _, u := range reject {
		t.Run("reject/"+u, func(t *testing.T) {
			if err := ValidateHTTPSURL(u); err == nil {
				t.Errorf("ValidateHTTPSURL(%q) = nil, want error", u)
			}
		})
	}
}

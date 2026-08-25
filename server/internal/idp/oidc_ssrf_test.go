package idp

import "testing"

func TestSSRFSafeDialControl(t *testing.T) {
	blocked := []string{
		"127.0.0.1:443", "10.0.0.5:443", "192.168.1.1:80", "172.16.0.1:443",
		"169.254.169.254:80",
		"100.64.0.1:443", "100.127.255.254:443",
		"[::1]:443", "[fe80::1]:443", "[fc00::1]:443", "0.0.0.0:443",
	}
	for _, a := range blocked {
		if err := ssrfSafeDialControl("tcp", a, nil); err == nil {
			t.Errorf("expected internal address %s to be blocked", a)
		}
	}
	allowed := []string{"8.8.8.8:443", "1.1.1.1:443", "[2606:4700:4700::1111]:443"}
	for _, a := range allowed {
		if err := ssrfSafeDialControl("tcp", a, nil); err != nil {
			t.Errorf("expected public address %s to be allowed, got %v", a, err)
		}
	}

	for _, a := range []string{"not-an-address", "hostname:443", "1.2.3.4"} {
		if err := ssrfSafeDialControl("tcp", a, nil); err == nil {
			t.Errorf("expected malformed address %q to error (fail closed)", a)
		}
	}
}

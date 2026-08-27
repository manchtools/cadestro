//go:build linux

package inventory

import (
	"testing"
)

func TestParsers_ScanError(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := parseCPUInfo(dir); err == nil {
		t.Error("parseCPUInfo(dir) = nil error, want a scan error")
	}
	if _, err := parseMemTotal(dir); err == nil {
		t.Error("parseMemTotal(dir) = nil error, want a scan error")
	}
	if _, err := parseOSRelease(dir); err == nil {
		t.Error("parseOSRelease(dir) = nil error, want a scan error")
	}
}

func TestParseMemTotal_EdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"MemTotal with no value", "MemTotal:\n"},
		{"MemTotal value not a number", "MemTotal:       notanumber kB\n"},
		{"no MemTotal line at all", "MemFree:         8192000 kB\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseMemTotal(writeTemp(t, c.content)); err == nil {
				t.Errorf("parseMemTotal(%q) = nil error, want failure", c.content)
			}
		})
	}
}

func TestParseOSReleaseLine_NoEquals(t *testing.T) {

	if _, _, ok := parseOSReleaseLine("this is not a property"); ok {
		t.Error("parseOSReleaseLine accepted a line with no '='")
	}
}

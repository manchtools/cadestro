package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeForLog(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		contains string
		excludes []string
	}{
		{
			name:     "empty stays empty",
			in:       "",
			contains: "",
		},
		{
			name:     "short ok line passes through",
			in:       "all good",
			contains: "all good",
		},
		{
			name:     "long line gets truncation marker",
			in:       strings.Repeat("x", 1024),
			contains: "[truncated by agent log filter]",
		},
		{
			name:     "enc:v1 ciphertext redacted",
			in:       "client_secret = enc:v1:abcdefGHIJklmn+/== rest of line",
			contains: "[REDACTED-ENC]",
			excludes: []string{"abcdefGHIJklmn", "enc:v1:"},
		},
		{
			name:     "multiple enc:v1 markers all redacted",
			in:       "a=enc:v1:AAA b=enc:v1:BBBccc done",
			contains: "[REDACTED-ENC]",
			excludes: []string{"AAA", "BBBccc"},
		},
		{
			name:     "enc:v1 followed by non-base64 char stops cleanly",
			in:       "before enc:v1:abc!end",
			contains: "[REDACTED-ENC]",
			excludes: []string{"enc:v1:abc"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := sanitizeForLog(tc.in)
			if tc.contains != "" {
				assert.Contains(t, out, tc.contains)
			}
			for _, ex := range tc.excludes {
				assert.NotContains(t, out, ex, "redacted output must not contain %q", ex)
			}
			assert.LessOrEqual(t, len(out), maxLogOutputBytes+len("... [truncated by agent log filter]"),
				"length never exceeds cap + marker")
		})
	}
}

func TestSanitizeForLog_PlaintextSecretBoundary(t *testing.T) {

	plaintext := "hunter2-luks-pass"
	out := sanitizeForLog("recovered key: " + plaintext)
	assert.Contains(t, out, plaintext,
		"a plaintext secret with no enc:v1: prefix is length-bounded only — the filter does not redact it")
	assert.NotContains(t, out, "[REDACTED-ENC]",
		"there is no enc:v1: token here, so nothing is redacted")
}

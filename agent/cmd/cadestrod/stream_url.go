package main

import (
	"fmt"
	"net/url"
	"strings"
)

func requireHTTPSAgentAddr(addr string) error {
	trimmed := strings.TrimSpace(addr)
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.ToLower(parsed.Scheme) != "https" || parsed.Opaque != "" || parsed.Host == "" {
		return fmt.Errorf("refusing non-https or hostless control URL %q: agent requires https://host for control connections", addr)
	}
	return nil
}

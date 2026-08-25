package dns

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// ErrInvalidConfig is returned when a Config field is unsafe or malformed for a
// resolver backend. Apply validates before touching any backend, so a bad config
// is rejected without side effects.
var ErrInvalidConfig = errors.New("dns: invalid config")

var validInterface = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@-]{0,14}$`)

var validDomainLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func validateConfig(cfg Config) error {
	for _, ns := range cfg.Nameservers {
		ip := net.ParseIP(ns)
		if ip == nil {
			return fmt.Errorf("%w: nameserver %q is not a valid IP address", ErrInvalidConfig, ns)
		}

		if ip.IsUnspecified() {
			return fmt.Errorf("%w: nameserver %q is the unspecified address and cannot be a resolver", ErrInvalidConfig, ns)
		}
	}
	for _, d := range cfg.SearchDomains {
		if err := validateDomain(d); err != nil {
			return err
		}
	}
	if cfg.Interface != "" && !validInterface.MatchString(cfg.Interface) {
		return fmt.Errorf("%w: interface %q is not a valid interface name", ErrInvalidConfig, cfg.Interface)
	}
	return nil
}

func validateDomain(d string) error {
	if d == "" {
		return fmt.Errorf("%w: empty search domain", ErrInvalidConfig)
	}
	if len(d) > 253 {
		return fmt.Errorf("%w: search domain %q exceeds 253 characters", ErrInvalidConfig, d)
	}
	if strings.ContainsAny(d, " \t\n\r\x00") {
		return fmt.Errorf("%w: search domain %q contains whitespace or control characters", ErrInvalidConfig, d)
	}

	labels := strings.Split(strings.TrimSuffix(d, "."), ".")
	for _, l := range labels {
		if !validDomainLabel.MatchString(l) {
			return fmt.Errorf("%w: search domain %q has an invalid label %q", ErrInvalidConfig, d, l)
		}
	}
	return nil
}

func partitionByFamily(nameservers []string) (v4, v6 []string) {
	for _, ns := range nameservers {
		ip := net.ParseIP(ns)
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			v4 = append(v4, ns)
		} else {
			v6 = append(v6, ns)
		}
	}
	return v4, v6
}

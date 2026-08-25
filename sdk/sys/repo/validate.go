package repo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/manchtools/cadestro/sdk/pkg"
)

var (
	validName            = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	validAptDistribution = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	validAptComponent    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	validAptArch         = regexp.MustCompile(`^[a-z0-9][a-z0-9,_-]*$`)
	validPacmanSigLevel  = regexp.MustCompile(`^[a-zA-Z ]+$`)
)

const maxNameLen = 128

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("%w: name exceeds %d characters", ErrInvalidName, maxNameLen)
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("%w: name must match [a-zA-Z0-9][a-zA-Z0-9._-]*", ErrInvalidName)
	}
	return nil
}

func rejectControl(field, s string) error {
	if hasControl(s) {
		return fmt.Errorf("%w: field %q contains a control character", ErrInvalidConfig, field)
	}
	return nil
}

func badShape(field string) error {
	return fmt.Errorf("%w: field %q has an invalid shape", ErrInvalidConfig, field)
}

// Validate checks the name and the configuration for this Manager's backend.
// A sub-config for a different backend is ignored. A name-only Repository
// (no sub-config) validates the name alone — the shape Remove uses.
func (m *manager) Validate(r Repository) error {
	if err := validateName(r.Name); err != nil {
		return err
	}
	switch m.b {
	case pkg.Apt:
		if r.Apt != nil {
			return validateApt(r.Apt)
		}
	case pkg.Dnf, pkg.Dnf5:
		if r.Dnf != nil {
			return validateDnf(r.Dnf)
		}
	case pkg.Pacman:
		if err := validatePacmanName(r.Name); err != nil {
			return err
		}
		if r.Pacman != nil {
			return validatePacman(r.Pacman)
		}
	case pkg.Zypper:
		if r.Zypper != nil {
			return validateZypper(r.Zypper)
		}
	}
	return nil
}

func validateAptURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: field %q is required", ErrInvalidConfig, "apt.url")
	}
	for _, r := range rawURL {
		if r <= ' ' || r == 0x7f {
			return badShape("apt.url")
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return badShape("apt.url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return badShape("apt.url")
	}
	if u.Host == "" {
		return badShape("apt.url")
	}
	if u.User != nil {
		return badShape("apt.url")
	}
	return nil
}

func validateApt(c *AptConfig) error {
	if err := validateAptURL(c.URL); err != nil {
		return err
	}
	if err := rejectControl("apt.distribution", c.Distribution); err != nil {
		return err
	}
	if c.Distribution != "" && !validAptDistribution.MatchString(c.Distribution) {
		return badShape("apt.distribution")
	}

	if c.Distribution == "" && len(c.Components) > 0 {
		return fmt.Errorf("%w: apt.components requires apt.distribution (a flat repository — empty distribution — must have no components)", ErrInvalidConfig)
	}
	for _, comp := range c.Components {
		if err := rejectControl("apt.components", comp); err != nil {
			return err
		}
		if !validAptComponent.MatchString(comp) {
			return badShape("apt.components")
		}
	}
	if err := rejectControl("apt.arch", c.Arch); err != nil {
		return err
	}
	if c.Arch != "" && !validAptArch.MatchString(c.Arch) {
		return badShape("apt.arch")
	}

	return nil
}

func validateDnf(c *DnfConfig) error {
	if c.BaseURL == "" {
		return fmt.Errorf("%w: field %q is required", ErrInvalidConfig, "dnf.baseurl")
	}
	if err := rejectControl("dnf.description", c.Description); err != nil {
		return err
	}

	if pkg.ValidateRepoBaseURL(c.BaseURL) != nil {
		return badShape("dnf.baseurl")
	}
	if c.GPGKey != "" && pkg.ValidateGpgKeyRef(c.GPGKey) != nil {
		return badShape("dnf.gpgkey")
	}
	return nil
}

func validatePacman(c *PacmanConfig) error {
	if c.Server == "" {
		return fmt.Errorf("%w: field %q is required", ErrInvalidConfig, "pacman.server")
	}
	if err := rejectControl("pacman.sig_level", c.SigLevel); err != nil {
		return err
	}
	if c.SigLevel != "" && !validPacmanSigLevel.MatchString(c.SigLevel) {
		return badShape("pacman.sig_level")
	}

	if disablesPacmanSig(c.SigLevel) {
		return fmt.Errorf("%w: field %q disables signature verification (Never)", ErrInvalidConfig, "pacman.sig_level")
	}
	if pkg.ValidateRepoBaseURL(c.Server) != nil {
		return badShape("pacman.server")
	}
	return nil
}

const pacmanReserved = "options"

func validatePacmanName(name string) error {
	if strings.EqualFold(name, pacmanReserved) {
		return fmt.Errorf("%w: %q is the reserved pacman.conf section, not a repository", ErrInvalidConfig, name)
	}
	return nil
}

func disablesPacmanSig(sigLevel string) bool {
	for _, tok := range strings.Fields(sigLevel) {
		switch strings.ToLower(tok) {
		case "never", "packagenever", "databasenever":
			return true
		}
	}
	return false
}

func validateZypper(c *ZypperConfig) error {
	if c.URL == "" {
		return fmt.Errorf("%w: field %q is required", ErrInvalidConfig, "zypper.url")
	}
	if err := rejectControl("zypper.description", c.Description); err != nil {
		return err
	}
	if err := rejectControl("zypper.type", c.Type); err != nil {
		return err
	}
	if !validZypperType(c.Type) {
		return badShape("zypper.type")
	}
	if pkg.ValidateRepoBaseURL(c.URL) != nil {
		return badShape("zypper.url")
	}
	if c.GPGKey != "" && pkg.ValidateGpgKeyRef(c.GPGKey) != nil {
		return badShape("zypper.gpgkey")
	}
	return nil
}

func validZypperType(s string) bool {
	switch s {
	case "", "rpm-md", "yast2", "plaindir":
		return true
	default:
		return false
	}
}

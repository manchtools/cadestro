package firewall

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/fs"
)

type firewalld struct {
	base
}

var _ Manager = (*firewalld)(nil)

const firewalldServicesDir = "/etc/firewalld/services"

func firewalldServiceName(namespace, id string) string {
	return namespace + "-" + id
}

// ApplyRule installs or updates rule. Writes the service XML, reloads firewalld
// so the new definition is recognised, and adds it to the default zone (no-op if
// already present).
//
// The XML lands before the three firewall-cmd calls, so a failure in any of them
// would otherwise leave the file behind — and firewalld parses every file in the
// services dir on the next reload by anything at all, so a rule the caller was
// told had failed would quietly become a loadable service definition. ApplyRule
// therefore deletes the file it wrote before returning the error.
//
// That cleanup is CREATE-ONLY, and the asymmetry is deliberate. ApplyRule also
// updates an existing rule, overwriting a service that may still be enabled in
// the zone; deleting on failure there would destroy a working definition and
// turn a failed update into an outage. So the file is removed only when this
// call brought it into existence.
//
// Ownership is established BY the write, not by a probe before it: the first
// attempt is an exclusive create, and only if that reports fs.ErrExists do we
// fall back to an ordinary overwrite, marked not-owned. Probing first would be a
// check-then-write race — a foreign definition appearing in the gap would be
// silently overwritten and then, on failure, deleted.
func (f *firewalld) ApplyRule(ctx context.Context, rule Rule) error {
	if err := validateRule(rule); err != nil {
		return err
	}
	if err := firewalldValidateRule(rule); err != nil {
		return err
	}
	zone, err := f.firewalldDefaultZone(ctx)
	if err != nil {
		return err
	}
	svc := firewalldServiceName(f.ns, rule.ID)
	xml := firewalldServiceXML(f.ns, rule)
	path := filepath.Join(firewalldServicesDir, svc+".xml")

	opts := fs.WriteOptions{Mode: 0o644, Owner: "root", Group: "root"}

	created := true
	if err := f.fsm.WriteFileExclusive(ctx, path, []byte(xml), opts); err != nil {
		if !errors.Is(err, fs.ErrExists) {
			return fmt.Errorf("write service xml %s: %w", path, err)
		}
		created = false
		if err := f.fsm.WriteFile(ctx, path, []byte(xml), opts); err != nil {
			return fmt.Errorf("write service xml %s: %w", path, err)
		}
	}

	if _, err := f.run(ctx, "firewall-cmd", "--reload"); err != nil {
		return f.discardCreatedServiceXML(ctx, path, created, fmt.Errorf("firewall-cmd --reload: %w", err))
	}

	if _, err := f.run(ctx, "firewall-cmd",
		"--permanent", "--zone="+zone, "--add-service="+svc,
	); err != nil {
		return f.discardCreatedServiceXML(ctx, path, created, fmt.Errorf("firewall-cmd add-service: %w", err))
	}

	if _, err := f.run(ctx, "firewall-cmd", "--reload"); err != nil {
		return f.discardCreatedServiceXML(ctx, path, created, fmt.Errorf("firewall-cmd --reload (post-enable): %w", err))
	}
	return nil
}

func (f *firewalld) discardCreatedServiceXML(ctx context.Context, path string, created bool, applyErr error) error {
	if !created {
		return applyErr
	}
	if err := f.fsm.Remove(ctx, path); err != nil {
		return fmt.Errorf("%w (the new service xml %s could NOT be removed: %v, so firewalld will parse it on the next reload)", applyErr, path, err)
	}
	return fmt.Errorf("%w (removed the new service xml %s)", applyErr, path)
}

// RemoveRule disables the service in the default zone and deletes its XML file.
// Missing services / files are no-ops, matching the idempotency contract.
func (f *firewalld) RemoveRule(ctx context.Context, id string) error {
	if err := validateRuleID(id); err != nil {
		return err
	}
	zone, err := f.firewalldDefaultZone(ctx)
	if err != nil {
		return err
	}
	svc := firewalldServiceName(f.ns, id)

	enabled, err := f.firewalldServiceIsEnabled(ctx, zone, svc)
	if err != nil {
		return fmt.Errorf("firewall-cmd list-services: %w", err)
	}
	if enabled {
		if _, err := f.run(ctx, "firewall-cmd",
			"--permanent", "--zone="+zone, "--remove-service="+svc,
		); err != nil {
			return fmt.Errorf("firewall-cmd --remove-service: %w", err)
		}
	}
	path := filepath.Join(firewalldServicesDir, svc+".xml")

	if err := f.fsm.Remove(ctx, path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if _, err := f.run(ctx, "firewall-cmd", "--reload"); err != nil {
		return fmt.Errorf("firewall-cmd --reload: %w", err)
	}
	return nil
}

func (f *firewalld) firewalldServiceIsEnabled(ctx context.Context, zone, svc string) (bool, error) {
	res, err := f.run(ctx, "firewall-cmd",
		"--permanent", "--zone="+zone, "--list-services",
	)
	if err != nil {
		return false, err
	}
	for _, field := range strings.Fields(res.Stdout) {
		if field == svc {
			return true, nil
		}
	}
	return false, nil
}

// List returns every managed service enabled in the default zone whose name
// starts with `<namespace>-`, reconstructed into Rule structs by reading each
// service's XML body.
func (f *firewalld) List(ctx context.Context) ([]Rule, error) {
	zone, err := f.firewalldDefaultZone(ctx)
	if err != nil {
		return nil, err
	}
	res, err := f.run(ctx, "firewall-cmd",
		"--permanent", "--zone="+zone, "--list-services",
	)
	if err != nil {
		return nil, fmt.Errorf("firewall-cmd list-services: %w", err)
	}
	ids := firewalldFilterNamespaceServices(res.Stdout, f.ns)
	rules := make([]Rule, 0, len(ids))
	for _, id := range ids {
		rule, ok := firewalldReadServiceRule(f.ns, id)
		if !ok {

			continue
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func firewalldValidateRule(rule Rule) error {
	if !rule.Allow {
		return fmt.Errorf("%w: deny rules not supported by firewalld backend in v1 (use nftables)", ErrInvalidRule)
	}
	if rule.Source != "" {
		return fmt.Errorf("%w: source scoping not supported by firewalld backend in v1 (use nftables)", ErrInvalidRule)
	}
	if rule.Dest != "" {
		return fmt.Errorf("%w: destination scoping not supported by firewalld backend in v1 (use nftables)", ErrInvalidRule)
	}
	if rule.Protocol != ProtocolTCP && rule.Protocol != ProtocolUDP {
		return fmt.Errorf("%w: firewalld backend requires a concrete protocol (tcp or udp)", ErrInvalidRule)
	}
	if rule.Port <= 0 {
		return fmt.Errorf("%w: firewalld backend requires Port > 0", ErrInvalidRule)
	}
	return nil
}

func firewalldServiceXML(namespace string, rule Rule) string {
	return strings.Join([]string{
		`<?xml version="1.0" encoding="utf-8"?>`,
		`<service>`,
		`  <short>` + firewalldServiceName(namespace, rule.ID) + `</short>`,
		`  <description>` + namespace + ` managed rule</description>`,
		fmt.Sprintf(`  <port port="%d" protocol="%s"/>`, rule.Port, rule.Protocol),
		`</service>`,
		``,
	}, "\n")
}

func firewalldFilterNamespaceServices(out, namespace string) []string {
	prefix := namespace + "-"
	fields := strings.Fields(out)
	var ids []string
	for _, f := range fields {
		if id, ok := strings.CutPrefix(f, prefix); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func firewalldReadServiceRule(namespace, id string) (Rule, bool) {
	path := filepath.Join(firewalldServicesDir, firewalldServiceName(namespace, id)+".xml")
	body, err := readFile(path)
	if err != nil {
		return Rule{}, false
	}

	rule := Rule{ID: id, Allow: true}
	if portIdx := strings.Index(string(body), `port port="`); portIdx >= 0 {
		rest := string(body)[portIdx+len(`port port="`):]
		end := strings.Index(rest, `"`)
		if end > 0 {
			if p, perr := strconv.Atoi(rest[:end]); perr == nil {
				rule.Port = p
			}
		}
	}
	if protoIdx := strings.Index(string(body), `protocol="`); protoIdx >= 0 {
		rest := string(body)[protoIdx+len(`protocol="`):]
		end := strings.Index(rest, `"`)
		if end > 0 {
			rule.Protocol = Protocol(rest[:end])
		}
	}
	if rule.Port == 0 || rule.Protocol == "" {
		return Rule{}, false
	}
	return rule, true
}

func (f *firewalld) firewalldDefaultZone(ctx context.Context) (string, error) {
	res, err := f.run(ctx, "firewall-cmd", "--get-default-zone")
	if err != nil {
		return "", fmt.Errorf("firewall-cmd --get-default-zone: %w", err)
	}
	zone := strings.TrimSpace(res.Stdout)
	if zone == "" {
		return "", fmt.Errorf("firewall-cmd --get-default-zone returned empty output")
	}
	return zone, nil
}

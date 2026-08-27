package service

import (
	"fmt"
	"path"
	"strings"
)

// ErrUnsafeUnitContent is returned by WriteUnit when the unit body carries a
// directive that would turn the agent into a root persistence / dropper
// primitive. A unit file written to /etc/systemd/system is executed by PID 1 as
// root, so its content is as privileged as the executables it names — the
// content policy below is the gate that keeps an attacker-supplied unit from
// running `curl | sh`, preloading a hostile library into every service, or
// executing a payload out of a world-writable directory.
var ErrUnsafeUnitContent = fmt.Errorf("unsafe systemd unit content")

var execDirectives = map[string]struct{}{
	"execstart":     {},
	"execstartpre":  {},
	"execstartpost": {},
	"execstop":      {},
	"execstoppost":  {},
	"execreload":    {},
	"execcondition": {},
}

var shellInterpreters = map[string]struct{}{
	"sh": {}, "bash": {}, "dash": {}, "zsh": {}, "ksh": {}, "ash": {}, "busybox": {},
}

var untrustedExecPrefixes = []string{"/tmp/", "/var/tmp/", "/dev/shm/"}

var dangerousEnvVars = map[string]struct{}{
	"LD_PRELOAD":      {},
	"LD_LIBRARY_PATH": {},
	"LD_AUDIT":        {},
}

func validateUnitContent(content string) error {

	for _, raw := range joinContinuationLines(content) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		value := strings.TrimSpace(line[eq+1:])

		switch {
		case isExecDirective(key):
			if err := validateExecLine(key, value); err != nil {
				return err
			}
		case key == "environmentfile":

			p := path.Clean(strings.TrimSpace(strings.TrimPrefix(value, "-")))
			for _, prefix := range untrustedExecPrefixes {
				if strings.HasPrefix(p, prefix) {
					return fmt.Errorf("%w: EnvironmentFile %q references a world-writable path", ErrUnsafeUnitContent, p)
				}
			}
		case key == "environment":
			if err := validateEnvLine(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func isExecDirective(key string) bool {
	_, ok := execDirectives[key]
	return ok
}

func joinContinuationLines(content string) []string {
	var out []string
	var pending strings.Builder
	continuing := false
	for _, l := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(l, " \t")
		if trailingBackslashes(trimmed)%2 == 1 {
			pending.WriteString(strings.TrimSuffix(trimmed, `\`))
			pending.WriteByte(' ')
			continuing = true
			continue
		}
		if continuing {
			pending.WriteString(l)
			out = append(out, pending.String())
			pending.Reset()
			continuing = false
		} else {
			out = append(out, l)
		}
	}
	if continuing {
		out = append(out, pending.String())
	}
	return out
}

func trailingBackslashes(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n
}

func validateExecLine(key, value string) error {

	cmd := strings.TrimLeft(value, "-@+!:")
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}
	fields := strings.Fields(cmd)
	exe := fields[0]
	base := path.Base(exe)

	if _, isShell := shellInterpreters[base]; isShell {
		for _, arg := range fields[1:] {
			if isShellCFlag(arg) {
				return fmt.Errorf("%w: %s shells out via %q -c (inline command execution)", ErrUnsafeUnitContent, key, base)
			}
		}
	}

	cleanExe := path.Clean(exe)
	for _, prefix := range untrustedExecPrefixes {
		if strings.HasPrefix(cleanExe, prefix) {
			return fmt.Errorf("%w: %s runs %q from a world-writable directory", ErrUnsafeUnitContent, key, exe)
		}
	}
	return nil
}

func isShellCFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return false
	}
	return strings.ContainsRune(arg[1:], 'c')
}

func validateEnvLine(value string) error {

	for _, pair := range strings.Fields(value) {
		eq := strings.IndexByte(pair, '=')
		name := pair
		if eq >= 0 {
			name = pair[:eq]
		}
		name = strings.Trim(strings.TrimSpace(name), `"'`)
		if _, bad := dangerousEnvVars[strings.ToUpper(name)]; bad {
			return fmt.Errorf("%w: sets %s, a dynamic-linker override", ErrUnsafeUnitContent, name)
		}
	}
	return nil
}

// ValidateUnitContent applies the unit-file content policy to content
// without writing anything — the same gate WriteUnit enforces. Exported
// so a caller that RENDERS unit content (the agent's embedded unit,
// spec 27) can pin at test time that its template never produces a
// directive the write path would reject.
func ValidateUnitContent(content string) error {
	return validateUnitContent(content)
}

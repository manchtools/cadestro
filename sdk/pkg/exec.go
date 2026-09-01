package pkg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func runRead(ctx context.Context, r sysexec.Runner, name string, args ...string) (sysexec.Result, error) {
	return r.Run(ctx, sysexec.Command{Name: name, Args: args})
}

func probe(ctx context.Context, r sysexec.Runner, name string, args ...string) (string, bool, error) {
	res, err := runRead(ctx, r, name, args...)
	if err != nil {
		return "", false, err
	}
	return res.Stdout, res.ExitCode == 0, nil
}

func runPriv(ctx context.Context, r sysexec.Runner, escalate bool, env []string, name string, args ...string) (sysexec.Result, error) {
	return r.Run(ctx, sysexec.Command{
		Name:     name,
		Args:     args,
		Env:      env,
		Escalate: escalate,
	})
}

func readOut(ctx context.Context, r sysexec.Runner, name string, args ...string) (string, error) {
	res, err := runRead(ctx, r, name, args...)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", &sysexec.CommandError{Name: name, ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return res.Stdout, nil
}

func runPrivStdin(ctx context.Context, r sysexec.Runner, escalate bool, env []string, stdin, name string, args ...string) (sysexec.Result, error) {
	var in io.Reader
	if stdin != "" {
		in = strings.NewReader(stdin)
	}
	return r.Run(ctx, sysexec.Command{
		Name:     name,
		Args:     args,
		Env:      env,
		Stdin:    in,
		Escalate: escalate,
	})
}

func asCommandError(name string, res sysexec.Result) error {
	if res.ExitCode == 0 {
		return nil
	}
	return &sysexec.CommandError{Name: name, ExitCode: res.ExitCode, Stderr: res.Stderr}
}

func rpmLocalPackageInfo(ctx context.Context, r sysexec.Runner, path string) (*LocalPackage, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, r, "rpm", "-qp", "--qf", "%{NAME}\n%{VERSION}-%{RELEASE}\n%{ARCH}", path)
	if err != nil {
		return nil, err
	}
	fields := splitPositionalFields(out)
	if len(fields) == 0 {
		return nil, fmt.Errorf("pkg: rpm -qp reported no name for %q", path)
	}
	name := fields[0]
	if err := ValidateRpmPackageName(name); err != nil {
		return nil, fmt.Errorf("pkg: local .rpm reports an unsafe package name: %w", err)
	}
	info := &LocalPackage{Name: name}
	if len(fields) > 1 {
		info.Version = fields[1]
	}
	if len(fields) > 2 {
		info.Arch = fields[2]
	}
	return info, nil
}

func parseColonValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

type sizeUnit struct {
	suffix string
	mult   int64
}

func parseSizeWithUnits(s string, units []sizeUnit) (size int64, ok bool) {
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			multiplier = u.mult
			s = strings.TrimSuffix(s, u.suffix)
			break
		}
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}

	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, false
	}
	scaled := n * float64(multiplier)

	if math.IsInf(scaled, 0) || scaled >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(scaled), true
}

func parseBinarySize(s string) (int64, bool) {
	return parseSizeWithUnits(s, []sizeUnit{
		{" KiB", 1024},
		{" MiB", 1024 * 1024},
		{" GiB", 1024 * 1024 * 1024},
		{" B", 1},
	})
}

func splitPositionalFields(data string) []string {
	lines := strings.Split(strings.TrimRight(data, "\n"), "\n")
	fields := make([]string, len(lines))
	for i, line := range lines {
		fields[i] = strings.TrimSpace(line)
	}
	return fields
}

func countNonEmptyLines(data string) int {
	count := 0
	for _, line := range bytes.Split([]byte(data), []byte("\n")) {
		if len(strings.TrimSpace(string(line))) > 0 {
			count++
		}
	}
	return count
}

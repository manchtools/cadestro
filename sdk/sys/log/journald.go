package log

import (
	"context"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

type journaldSource struct {
	r exec.Runner
}

// Query builds and runs a journalctl invocation. Every dynamic value is an
// option-ARGUMENT (`-u <unit>`, `--grep <pat>`, …), never a positional operand,
// so none can be reinterpreted as a flag. Filters are validated first.
func (s *journaldSource) Query(ctx context.Context, q Query) ([]string, error) {
	if err := validateQuery(q); err != nil {
		return nil, err
	}
	args := []string{"--no-pager", "-n", strconv.Itoa(cappedLines(q.Lines))}
	if q.Unit != "" {
		args = append(args, "-u", q.Unit)
	}
	if q.Since != "" {
		args = append(args, "--since", q.Since)
	}
	if q.Until != "" {
		args = append(args, "--until", q.Until)
	}
	if q.Priority != "" {
		args = append(args, "-p", q.Priority)
	}
	if q.Kernel {
		args = append(args, "-k")
	}
	if q.Grep != "" {
		args = append(args, "--grep", q.Grep)
	}
	res, err := s.r.Run(ctx, exec.Command{Name: "journalctl", Args: args, Escalate: true})
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {

		if q.Grep != "" && res.ExitCode == 1 && strings.TrimSpace(res.Stderr) == "" {
			return []string{}, nil
		}
		return nil, &exec.CommandError{Name: "journalctl", ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return dropStatusMarkers(splitLines(res.Stdout)), nil
}

func dropStatusMarkers(lines []string) []string {
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "-- ") && strings.HasSuffix(t, " --") {
			continue
		}
		kept = append(kept, ln)
	}
	return kept
}

func splitLines(out string) []string {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return []string{}
	}
	return strings.Split(out, "\n")
}

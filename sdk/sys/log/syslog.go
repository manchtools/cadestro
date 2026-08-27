package log

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

var syslogPaths = []string{"/var/log/syslog", "/var/log/messages"}

var statFile = os.Stat

type syslogSource struct {
	r exec.Runner
}

// Query tails the active syslog file and applies the grep filter.
func (s *syslogSource) Query(ctx context.Context, q Query) ([]string, error) {
	if err := validateQuery(q); err != nil {
		return nil, err
	}

	var re *regexp.Regexp
	if q.Grep != "" {
		var err error
		if re, err = regexp.Compile(q.Grep); err != nil {
			return nil, fmt.Errorf("%w: grep pattern: %v", ErrInvalidQuery, err)
		}
	}
	path, err := syslogPath()
	if err != nil {
		return nil, err
	}

	out, err := runEscalated(ctx, s.r, nil, "tail", "-n", strconv.Itoa(cappedLines(q.Lines)), "--", path)
	if err != nil {
		return nil, err
	}
	lines := splitLines(out)
	if re == nil {
		return lines, nil
	}
	matched := make([]string, 0, len(lines))
	for _, l := range lines {
		if re.MatchString(l) {
			matched = append(matched, l)
		}
	}
	return matched, nil
}

func syslogPath() (string, error) {
	for _, p := range syslogPaths {
		if _, err := statFile(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("log: no syslog file found (looked in %v)", syslogPaths)
}

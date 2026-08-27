// Package osquery integrates the osquery binary for system queries through an
// injected exec.Runner.
//
// Build a Querier with a Runner and call its methods; every query is escalated
// through the Runner. Every query path — the convenience table path AND raw
// SQL — refuses a curated deny-list of credential-bearing tables before
// running anything: a query that references a deny-listed table (in any
// clause) is rejected, so there is no path to read shadow/sudoers/… via
// osquery. Inputs are size-bounded in-package: table names and raw SQL beyond
// the caps are refused before execution.
//
//	r, _ := exec.NewRunner(exec.Sudo)
//	q, err := osquery.New(r) // ErrNotInstalled if osqueryi is absent
//	if err != nil { ... }
//	rows, err := q.QueryTable(ctx, "os_version")
//
// Results are SDK-native: a Row is one result row as column→value, exactly
// osquery's own --json output shape. Refusals are errors.Is-able sentinels
// (ErrTableNotPermitted, ErrInvalidTableName, ErrQueryTooLong); consumers that
// need a wire envelope build their own at their boundary.
//
// New is a single-implementation capability (design §3.8): it exposes the
// Querier interface for shape-uniformity with the backend-pattern packages even
// though osquery is the only implementation. There is no Backend argument — only
// the required Runner.
package osquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	osexec "os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

// Row is one osquery result row: column name → value. It is exactly the
// element shape of osqueryi's --json output.
type Row map[string]string

const (
	maxTableNameLen = 64
	maxRawSQLLen    = 4096
)

var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

var sensitiveTables = map[string]bool{
	"shadow":        true,
	"process_envs":  true,
	"crontab":       true,
	"shell_history": true,
	"sudoers":       true,
}

func isSensitiveTable(name string) bool {
	return sensitiveTables[strings.ToLower(strings.TrimSpace(name))]
}

func sensitiveTableRefIn(sql string) string {
	lower := strings.ToLower(sql)
	for name := range sensitiveTables {
		if containsWord(lower, name) {
			return name
		}
	}
	return ""
}

func containsWord(s, word string) bool {
	for from := 0; ; {
		i := strings.Index(s[from:], word)
		if i < 0 {
			return false
		}
		i += from
		leftOK := i == 0 || !isIdentByte(s[i-1])
		rightOK := i+len(word) >= len(s) || !isIdentByte(s[i+len(word)])
		if leftOK && rightOK {
			return true
		}
		from = i + 1
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

var (
	// ErrNotInstalled is returned when osquery is not installed on the system.
	ErrNotInstalled = errors.New("osquery is not installed")

	// ErrQueryFailed is returned when an osquery query fails.
	ErrQueryFailed = errors.New("osquery query failed")

	// ErrTableNotPermitted is the credential-table deny-list refusal: the
	// query referenced a table on the curated sensitiveTables list. The
	// wrapped error names the offending table.
	ErrTableNotPermitted = errors.New("table is not permitted")

	// ErrInvalidTableName is the shape refusal: the table name is not a safe
	// osquery identifier (or exceeds the name cap). Distinct from
	// ErrTableNotPermitted — shape vs policy.
	ErrInvalidTableName = errors.New("invalid table name")

	// ErrQueryTooLong is the size refusal: raw SQL exceeded maxRawSQLLen.
	ErrQueryTooLong = errors.New("osquery input exceeds size limit")

	osqueryPaths = []string{
		"/usr/bin/osqueryi",
		"/usr/local/bin/osqueryi",
		"/opt/osquery/bin/osqueryi",
	}

	defaultTimeout = 30 * time.Second
)

// Querier is the osquery surface: a small, ctx-first interface over the osquery
// binary. It is single-implementation by nature (§3.8) — there is no second way
// to run osquery — but it is an interface so a consumer learns the same
// construct-a-handle shape as every other capability.
type Querier interface {
	// IsInstalled reports, live, whether an osqueryi binary is currently
	// reachable. New already fails closed with ErrNotInstalled when the binary
	// is absent at construction; IsInstalled re-probes so a caller can detect
	// the binary being removed during the agent's lifetime. The ctx is accepted
	// for shape-uniformity; the probe itself is a filesystem lookup.
	IsInstalled(ctx context.Context) bool
	// ListTables returns the names of the available osquery tables.
	ListTables(ctx context.Context) ([]string, error)
	// QueryTable runs SELECT * FROM <table> after shape validation
	// (ErrInvalidTableName) and the credential-table deny-list
	// (ErrTableNotPermitted); both refuse before anything executes.
	QueryTable(ctx context.Context, tableName string) ([]Row, error)
	// QuerySQL runs raw SQL and parses the JSON result rows. It is gated by
	// the same credential-table deny-list as the table path — raw SQL
	// referencing a deny-listed table is refused (ErrTableNotPermitted)
	// before anything executes — and size-bounded (ErrQueryTooLong).
	QuerySQL(ctx context.Context, sql string) ([]Row, error)
}

type client struct {
	binaryPath string
	r          exec.Runner
}

// New creates an osquery Querier driven by runner. Returns ErrNotInstalled when
// the osqueryi binary is not found (eager fail-closed probe, so a caller learns
// at construction that osquery is unavailable), and an error when runner is nil.
func New(runner exec.Runner) (Querier, error) {
	if runner == nil {
		return nil, fmt.Errorf("osquery: %w", exec.ErrRunnerRequired)
	}
	path := findOsqueryBinary()
	if path == "" {
		return nil, ErrNotInstalled
	}
	return &client{binaryPath: path, r: runner}, nil
}

// IsInstalled re-probes for the osqueryi binary so callers can detect removal at
// runtime. See the Querier.IsInstalled contract.
func (c *client) IsInstalled(ctx context.Context) bool {
	return findOsqueryBinary() != ""
}

var lookPath = osexec.LookPath

func findOsqueryBinary() string {
	for _, path := range osqueryPaths {
		if _, err := lookPath(path); err == nil {
			return path
		}
	}
	if path, err := lookPath("osqueryi"); err == nil {
		return path
	}
	return ""
}

// ListTables returns a list of available osquery tables.
func (c *client) ListTables(ctx context.Context) ([]string, error) {
	output, err := c.execQuery(ctx, ".tables")
	if err != nil {
		return nil, err
	}

	var tables []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		name, ok := strings.CutPrefix(line, "=>")
		if !ok {
			continue
		}
		if name = strings.TrimSpace(name); name != "" {
			tables = append(tables, name)
		}
	}
	return tables, nil
}

var tableSQL = map[string]string{
	"authorized_keys": "SELECT authorized_keys.* FROM users JOIN authorized_keys USING (uid)",
}

// QuerySQL executes a raw SQL query against osquery. It is a public entry
// point, so the size cap and the credential-table deny-list gate it directly:
// refusal happens here, before any command runs. The gates deliberately sit
// above execQuery so ListTables' `.tables` meta-command stays unaffected.
func (c *client) QuerySQL(ctx context.Context, sql string) ([]Row, error) {
	if len(sql) > maxRawSQLLen {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrQueryTooLong, len(sql), maxRawSQLLen)
	}
	if name := sensitiveTableRefIn(sql); name != "" {
		return nil, fmt.Errorf("%w: %q", ErrTableNotPermitted, name)
	}
	output, err := c.execQuery(ctx, sql)
	if err != nil {
		return nil, err
	}

	var rows []Row
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, fmt.Errorf("failed to parse osquery output: %w", err)
	}
	return rows, nil
}

// QueryTable queries a specific table by name.
func (c *client) QueryTable(ctx context.Context, tableName string) ([]Row, error) {
	sql, ok := tableSQL[tableName]
	if !ok {
		if len(tableName) > maxTableNameLen || !validTableName.MatchString(tableName) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidTableName, tableName)
		}
		if isSensitiveTable(tableName) {
			return nil, fmt.Errorf("%w: %q", ErrTableNotPermitted, tableName)
		}
		sql = fmt.Sprintf("SELECT * FROM %s", tableName)
	}
	return c.QuerySQL(ctx, sql)
}

func (c *client) execQuery(ctx context.Context, query string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	args := []string{}
	if strings.HasPrefix(query, ".") {
		args = append(args, query)
	} else {
		args = append(args, "--json", query)
	}

	res, err := c.r.Run(ctx, exec.Command{Name: c.binaryPath, Args: args, Escalate: true})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	if res.ExitCode != 0 {
		if stderr := strings.TrimSpace(res.Stderr); stderr != "" {
			return "", fmt.Errorf("%w: %s", ErrQueryFailed, stderr)
		}
		return "", fmt.Errorf("%w: exit code %d", ErrQueryFailed, res.ExitCode)
	}

	return strings.TrimSpace(res.Stdout), nil
}

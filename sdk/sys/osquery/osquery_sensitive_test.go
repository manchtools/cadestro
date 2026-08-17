package osquery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

// The QueryTable path must refuse known-sensitive tables even though they
// match validTableName, and must do so BEFORE building or running any SQL.
// The sensitive cases are sourced from intent (credential-bearing osquery
// tables), not from the validTableName regex. The FakeRunner records every
// execution so the test can prove the deny path runs zero queries.
func TestQueryTable_DeniesSensitiveTables(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{Stdout: "[]"}, nil) // consumed only by the benign os_version run
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	ctx := context.Background()

	// present-but-wrong: each sensitive table is regex-valid but policy-forbidden.
	for table := range sensitiveTables {
		_, err := c.QueryTable(ctx, table)
		if !errors.Is(err, ErrTableNotPermitted) {
			t.Errorf("QueryTable(%q) err = %v, want ErrTableNotPermitted", table, err)
		}
		if err == nil || !strings.Contains(err.Error(), table) {
			t.Errorf("QueryTable(%q) err = %v, want it to name the refused table", table, err)
		}
	}

	// ABSENT: the empty table name is a shape rejection, not a policy one.
	if _, err := c.QueryTable(ctx, ""); !errors.Is(err, ErrInvalidTableName) {
		t.Errorf("QueryTable(\"\") err = %v, want ErrInvalidTableName", err)
	}

	// No query has run yet — every denied/invalid table short-circuits.
	if n := len(r.Calls()); n != 0 {
		t.Fatalf("a denied table must execute no query, but %d ran: %v", n, r.Calls())
	}

	// correct: a benign table builds and runs SELECT * FROM <table> exactly once.
	if _, err := c.QueryTable(ctx, "os_version"); err != nil {
		t.Fatalf("QueryTable(os_version) error: %v", err)
	}
	calls := r.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly one query, got %d: %v", len(calls), calls)
	}
	if argv := strings.Join(calls[0].Args, " "); !strings.Contains(argv, "SELECT * FROM os_version") || !calls[0].Escalate {
		t.Errorf("os_version query argv = %q (escalate=%v), want an escalated --json SELECT", argv, calls[0].Escalate)
	}
}

// Input caps: oversized identifiers and raw SQL are refused before any
// command runs, with the shape/size sentinels — not silently truncated and
// not passed through to osqueryi.
func TestInputCaps_RefusedBeforeExec(t *testing.T) {
	r := exectest.New(exec.Direct)
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	ctx := context.Background()

	longName := strings.Repeat("a", maxTableNameLen+1)
	if _, err := c.QueryTable(ctx, longName); !errors.Is(err, ErrInvalidTableName) {
		t.Errorf("QueryTable(%d-byte name) err = %v, want ErrInvalidTableName", len(longName), err)
	}

	longSQL := "SELECT '" + strings.Repeat("x", maxRawSQLLen) + "'"
	if _, err := c.QuerySQL(ctx, longSQL); !errors.Is(err, ErrQueryTooLong) {
		t.Errorf("QuerySQL(%d bytes) err = %v, want ErrQueryTooLong", len(longSQL), err)
	}

	// boundary: exactly at the caps is allowed through the shape/size gates.
	r.Push(exec.Result{Stdout: "[]"}, nil)
	exactName := strings.Repeat("a", maxTableNameLen)
	if _, err := c.QueryTable(ctx, exactName); err != nil {
		t.Errorf("QueryTable(%d-byte name) err = %v, want accepted at the cap", len(exactName), err)
	}

	if n := len(r.Calls()); n != 1 {
		t.Fatalf("over-cap inputs must execute no query; %d command(s) ran, want exactly the boundary control", n)
	}
}

// QuerySQL is a public entry point and must be gated by the same
// credential-table deny-list as Query's table and RawSql paths: the package
// promises there is NO path to read shadow/sudoers/… via osquery, and a direct
// QuerySQL call is such a path. Refusal must happen BEFORE any command runs.
func TestQuerySQL_GatedByDenyList(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{Stdout: `[{"hash":"x"}]`}, nil) // consumed only by the benign positive control
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	ctx := context.Background()

	// Every deny-listed table is refused with the policy sentinel, and the
	// error names it.
	for table := range sensitiveTables {
		_, err := c.QuerySQL(ctx, "SELECT * FROM "+table)
		if !errors.Is(err, ErrTableNotPermitted) || !strings.Contains(err.Error(), table) {
			t.Errorf("QuerySQL(FROM %s) err = %v, want ErrTableNotPermitted naming the table", table, err)
		}
	}

	// CTE smuggling cannot alias its way past the whole-word scan.
	if _, err := c.QuerySQL(ctx, "WITH stolen AS (SELECT * FROM shadow) SELECT * FROM stolen"); !errors.Is(err, ErrTableNotPermitted) {
		t.Errorf("CTE-smuggled shadow read err = %v, want ErrTableNotPermitted", err)
	}

	// Recorded decision, not an accident: the gate FAILS CLOSED on any
	// whole-word deny token even in a value position, so file *metadata*
	// about /etc/sudoers is also refused. Over-refusal is the safe direction
	// for a credential-table gate.
	if _, err := c.QuerySQL(ctx, "SELECT * FROM file WHERE path = '/etc/sudoers'"); !errors.Is(err, ErrTableNotPermitted) {
		t.Errorf("value-position sudoers err = %v, want the fail-closed refusal", err)
	}

	// Nothing above may have reached the Runner.
	if n := len(r.Calls()); n != 0 {
		t.Fatalf("refused raw SQL ran %d command(s) before the policy gate; want 0", n)
	}

	// Positive control: benign raw SQL still executes exactly once.
	if _, err := c.QuerySQL(ctx, "SELECT * FROM os_version"); err != nil {
		t.Fatalf("benign raw SQL must still run, got %v", err)
	}
	if n := len(r.Calls()); n != 1 {
		t.Fatalf("benign raw SQL ran %d command(s), want exactly 1", n)
	}
}

func TestExecQuery_Failures(t *testing.T) {
	t.Run("exec error", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		r.Push(exec.Result{}, errors.New("sudo: a password is required"))
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		if _, err := c.QuerySQL(context.Background(), "SELECT 1"); !errors.Is(err, ErrQueryFailed) {
			t.Errorf("err = %v, want ErrQueryFailed", err)
		}
	})
	t.Run("non-zero exit with stderr", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		r.Push(exec.Result{ExitCode: 1, Stderr: "no such table: bogus"}, nil)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		_, err := c.QuerySQL(context.Background(), "SELECT * FROM bogus")
		if !errors.Is(err, ErrQueryFailed) || !strings.Contains(err.Error(), "no such table") {
			t.Errorf("err = %v, want ErrQueryFailed naming the stderr", err)
		}
	})
	t.Run("non-zero exit no stderr", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		r.Push(exec.Result{ExitCode: 2}, nil)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		if _, err := c.QuerySQL(context.Background(), "SELECT 1"); !errors.Is(err, ErrQueryFailed) {
			t.Errorf("err = %v, want ErrQueryFailed", err)
		}
	})
	t.Run("unparseable JSON", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		r.Push(exec.Result{Stdout: "not json"}, nil)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		if _, err := c.QuerySQL(context.Background(), "SELECT 1"); err == nil ||
			!strings.Contains(err.Error(), "parse osquery output") {
			t.Errorf("err = %v, want a parse failure", err)
		}
	})
}

// Self-discovering guard: the deny-list must be non-empty, every member must be
// enforced by isSensitiveTable (including case/whitespace variants), and sample
// benign tables must stay queryable.
func TestSensitiveTables_PolicyNonEmptyAndEnforced(t *testing.T) {
	if len(sensitiveTables) == 0 {
		t.Fatal("the sensitive-table deny-list must not be empty")
	}
	for table := range sensitiveTables {
		if !isSensitiveTable(table) {
			t.Errorf("%q is in sensitiveTables but isSensitiveTable returns false", table)
		}
		if !isSensitiveTable(strings.ToUpper(table)) || !isSensitiveTable(" "+table+" ") {
			t.Errorf("%q case/whitespace variants must also be refused", table)
		}
	}
	for _, benign := range []string{"os_version", "uptime", "system_info"} {
		if isSensitiveTable(benign) {
			t.Errorf("%q is a benign table and must stay queryable", benign)
		}
	}
}

package osquery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestQueryTable_DeniesSensitiveTables(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{Stdout: "[]"}, nil)
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	ctx := context.Background()

	for table := range sensitiveTables {
		_, err := c.QueryTable(ctx, table)
		if !errors.Is(err, ErrTableNotPermitted) {
			t.Errorf("QueryTable(%q) err = %v, want ErrTableNotPermitted", table, err)
		}
		if err == nil || !strings.Contains(err.Error(), table) {
			t.Errorf("QueryTable(%q) err = %v, want it to name the refused table", table, err)
		}
	}

	if _, err := c.QueryTable(ctx, ""); !errors.Is(err, ErrInvalidTableName) {
		t.Errorf("QueryTable(\"\") err = %v, want ErrInvalidTableName", err)
	}

	if n := len(r.Calls()); n != 0 {
		t.Fatalf("a denied table must execute no query, but %d ran: %v", n, r.Calls())
	}

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

	r.Push(exec.Result{Stdout: "[]"}, nil)
	exactName := strings.Repeat("a", maxTableNameLen)
	if _, err := c.QueryTable(ctx, exactName); err != nil {
		t.Errorf("QueryTable(%d-byte name) err = %v, want accepted at the cap", len(exactName), err)
	}

	if n := len(r.Calls()); n != 1 {
		t.Fatalf("over-cap inputs must execute no query; %d command(s) ran, want exactly the boundary control", n)
	}
}

func TestQuerySQL_GatedByDenyList(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{Stdout: `[{"hash":"x"}]`}, nil)
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	ctx := context.Background()

	for table := range sensitiveTables {
		_, err := c.QuerySQL(ctx, "SELECT * FROM "+table)
		if !errors.Is(err, ErrTableNotPermitted) || !strings.Contains(err.Error(), table) {
			t.Errorf("QuerySQL(FROM %s) err = %v, want ErrTableNotPermitted naming the table", table, err)
		}
	}

	if _, err := c.QuerySQL(ctx, "WITH stolen AS (SELECT * FROM shadow) SELECT * FROM stolen"); !errors.Is(err, ErrTableNotPermitted) {
		t.Errorf("CTE-smuggled shadow read err = %v, want ErrTableNotPermitted", err)
	}

	if _, err := c.QuerySQL(ctx, "SELECT * FROM file WHERE path = '/etc/sudoers'"); !errors.Is(err, ErrTableNotPermitted) {
		t.Errorf("value-position sudoers err = %v, want the fail-closed refusal", err)
	}

	if n := len(r.Calls()); n != 0 {
		t.Fatalf("refused raw SQL ran %d command(s) before the policy gate; want 0", n)
	}

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

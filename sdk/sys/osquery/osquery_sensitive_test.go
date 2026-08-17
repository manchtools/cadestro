package osquery

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

// The non-raw Query() table path must refuse known-sensitive tables even though
// they match validTableName, and must do so BEFORE building or running any SQL.
// The sensitive cases are sourced from intent (credential-bearing osquery
// tables), not from the validTableName regex. The FakeRunner records every
// execution so the test can prove the deny path runs zero queries.
func TestQuery_DeniesSensitiveTables(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{Stdout: "[]"}, nil) // consumed only by the benign os_version run
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	ctx := context.Background()

	// present-but-wrong: each sensitive table is regex-valid but policy-forbidden.
	for table := range sensitiveTables {
		res, err := c.Query(ctx, &pb.OSQuery{QueryId: "q", Table: table})
		if err != nil {
			t.Fatalf("Query(%q) returned a transport error: %v", table, err)
		}
		if res.Success {
			t.Errorf("Query(%q) succeeded; a sensitive table must be refused", table)
		}
		if !strings.Contains(res.Error, table) || !strings.Contains(res.Error, "not permitted") {
			t.Errorf("Query(%q) error = %q, want it to name the table as not permitted", table, res.Error)
		}
	}

	// ABSENT: empty table and empty RawSql → the existing invalid-name rejection.
	res, err := c.Query(ctx, &pb.OSQuery{QueryId: "q", Table: ""})
	if err != nil {
		t.Fatalf("Query(empty) transport error: %v", err)
	}
	if res.Success || !strings.Contains(res.Error, "invalid table name") {
		t.Errorf("empty table = %+v, want the invalid-name rejection", res)
	}

	// No query has run yet — every denied/invalid table short-circuits.
	if n := len(r.Calls()); n != 0 {
		t.Fatalf("a denied table must execute no query, but %d ran: %v", n, r.Calls())
	}

	// correct: a benign table builds and runs SELECT * FROM <table> exactly once.
	res, err = c.Query(ctx, &pb.OSQuery{QueryId: "q", Table: "os_version"})
	if err != nil {
		t.Fatalf("Query(os_version) error: %v", err)
	}
	if !res.Success {
		t.Errorf("a non-sensitive table must be queryable, got %+v", res)
	}
	calls := r.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly one query, got %d: %v", len(calls), calls)
	}
	if argv := strings.Join(calls[0].Args, " "); !strings.Contains(argv, "SELECT * FROM os_version") || !calls[0].Escalate {
		t.Errorf("os_version query argv = %q (escalate=%v), want an escalated --json SELECT", argv, calls[0].Escalate)
	}
}

// RawSql is gated against the deny-list too: a raw query naming a sensitive
// table is refused (folded into the result) BEFORE osqueryi runs — there is no
// escape hatch around the credential-table policy. (Supersedes the prior WS4
// behaviour where signed RawSql bypassed the gate.)
func TestQuery_RawSqlGatedByDenyList(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{Stdout: `[{"hash":"x"}]`}, nil)
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	res, err := c.Query(context.Background(), &pb.OSQuery{QueryId: "q", RawSql: "SELECT * FROM shadow"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.Error == "" {
		t.Errorf("RawSql naming `shadow` must be refused by the deny-list, got %+v", res)
	}
	if n := len(r.Calls()); n != 0 {
		t.Errorf("refused RawSql ran %d command(s) before the policy gate; want 0", n)
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

	// Every deny-listed table is refused, and the error names it.
	for table := range sensitiveTables {
		_, err := c.QuerySQL(ctx, "SELECT * FROM "+table)
		if err == nil || !strings.Contains(err.Error(), "not permitted") || !strings.Contains(err.Error(), table) {
			t.Errorf("QuerySQL(FROM %s) err = %v, want a refusal naming the table as not permitted", table, err)
		}
	}

	// CTE smuggling cannot alias its way past the whole-word scan.
	if _, err := c.QuerySQL(ctx, "WITH stolen AS (SELECT * FROM shadow) SELECT * FROM stolen"); err == nil ||
		!strings.Contains(err.Error(), "not permitted") {
		t.Errorf("CTE-smuggled shadow read err = %v, want a deny-list refusal", err)
	}

	// Recorded decision, not an accident: the gate FAILS CLOSED on any
	// whole-word deny token even in a value position, so file *metadata*
	// about /etc/sudoers is also refused. Over-refusal is the safe direction
	// for a credential-table gate.
	if _, err := c.QuerySQL(ctx, "SELECT * FROM file WHERE path = '/etc/sudoers'"); err == nil ||
		!strings.Contains(err.Error(), "not permitted") {
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

package osquery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestListTables(t *testing.T) {
	r := exectest.New(exec.Direct)
	// REAL `osqueryi .tables` format: one table per line as "  => <name>"
	// (leading whitespace + "=> " prefix). The "=> " lines ARE the data — they
	// are the table names, not noise to skip. Blank lines are ignored. This
	// mirrors the captured real output the sys/osquery container test asserts
	// live; an earlier fake fed bare names + treated "=>" lines as skippable,
	// which inverted the contract and hid that the parser dropped every real
	// table.
	r.Push(exec.Result{Stdout: "  => os_version\n  => uptime\n\n  => system_info\n"}, nil)
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	tables, err := c.ListTables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"os_version": true, "uptime": true, "system_info": true}
	if len(tables) != len(want) {
		t.Fatalf("tables = %v, want %d entries", tables, len(want))
	}
	for _, tb := range tables {
		if !want[tb] {
			t.Errorf("unexpected table %q", tb)
		}
		delete(want, tb)
	}
	if len(want) != 0 {
		t.Errorf("parser dropped tables: %v", want)
	}
	// `.tables` is a dot-command — passed bare, not via --json.
	if argv := strings.Join(r.Calls()[0].Args, " "); argv != ".tables" {
		t.Errorf("argv = %q, want bare `.tables`", argv)
	}
}

func TestListTables_ExecError(t *testing.T) {
	r := exectest.New(exec.Direct)
	r.Push(exec.Result{ExitCode: 1, Stderr: "boom"}, nil)
	c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
	if _, err := c.ListTables(context.Background()); err == nil {
		t.Error("ListTables swallowed a query failure")
	}
}

func TestQueryTable(t *testing.T) {
	t.Run("benign", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		r.Push(exec.Result{Stdout: `[{"name":"x"}]`}, nil)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		rows, err := c.QueryTable(context.Background(), "os_version")
		if err != nil || len(rows) != 1 {
			t.Fatalf("QueryTable = (%v,%v), want one row", rows, err)
		}
		if rows[0]["name"] != "x" {
			t.Errorf("row = %v, want the name column decoded", rows[0])
		}
	})
	t.Run("custom tableSQL", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		r.Push(exec.Result{Stdout: "[]"}, nil)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		if _, err := c.QueryTable(context.Background(), "authorized_keys"); err != nil {
			t.Fatal(err)
		}
		if argv := strings.Join(r.Calls()[0].Args, " "); !strings.Contains(argv, "JOIN authorized_keys") {
			t.Errorf("custom SQL not used: %q", argv)
		}
	})
	t.Run("invalid name rejected before exec", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		if _, err := c.QueryTable(context.Background(), "bad name!"); !errors.Is(err, ErrInvalidTableName) {
			t.Errorf("err = %v, want ErrInvalidTableName", err)
		}
		if len(r.Calls()) != 0 {
			t.Error("ran a query for an invalid table name")
		}
	})
	t.Run("sensitive table refused before exec", func(t *testing.T) {
		r := exectest.New(exec.Direct)
		c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
		if _, err := c.QueryTable(context.Background(), "shadow"); !errors.Is(err, ErrTableNotPermitted) {
			t.Errorf("err = %v, want ErrTableNotPermitted", err)
		}
		if len(r.Calls()) != 0 {
			t.Error("ran a query for a sensitive table")
		}
	})
}

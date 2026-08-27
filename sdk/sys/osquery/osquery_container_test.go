//go:build container

package osquery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func realQuerier(t *testing.T) Querier {
	t.Helper()
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	q, err := New(r)
	if err != nil {

		t.Skipf("osquery not installed here: %v", err)
	}
	return q
}

func osqCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestQueryTable_OSVersion_Container(t *testing.T) {
	rows, err := realQuerier(t).QueryTable(osqCtx(t), "os_version")
	if err != nil {
		t.Fatalf("QueryTable(os_version): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("os_version returned %d rows, want 1", len(rows))
	}
	if name := rows[0]["name"]; name == "" {
		t.Errorf("os_version row missing/empty `name` column: %+v", rows[0])
	}
}

func TestIsInstalled_Container(t *testing.T) {
	if !realQuerier(t).IsInstalled(osqCtx(t)) {
		t.Error("IsInstalled = false, but osqueryi is installed in this image")
	}
}

func TestListTables_Container(t *testing.T) {
	tables, err := realQuerier(t).ListTables(osqCtx(t))
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("ListTables returned no tables from real osqueryi `.tables`")
	}
	for _, want := range []string{"os_version", "processes"} {
		if !containsTable(tables, want) {
			t.Errorf("ListTables missing core table %q; got %d tables", want, len(tables))
		}
	}
}

func TestDenyList_RefusedBeforeExec_Container(t *testing.T) {
	q := realQuerier(t)
	ctx := osqCtx(t)
	if len(sensitiveTables) == 0 {
		t.Fatal("sensitiveTables is empty — deny-list coverage would be vacuous")
	}
	for table := range sensitiveTables {

		_, err := q.QueryTable(ctx, table)
		if !errors.Is(err, ErrTableNotPermitted) || !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("QueryTable(%q): want the 'not permitted' policy refusal, got %v", table, err)
		}

		_, err = q.QuerySQL(ctx, "SELECT * FROM "+table)
		if !errors.Is(err, ErrTableNotPermitted) {
			t.Errorf("QuerySQL(FROM %s): want ErrTableNotPermitted, got %v", table, err)
		}
	}
}

var mustDenyTables = []string{"shadow", "process_envs", "shell_history", "crontab", "sudoers"}

func TestDenyList_ThreatModelComplete_Container(t *testing.T) {
	q := realQuerier(t)
	ctx := osqCtx(t)
	tables, err := q.ListTables(ctx)
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}

	checked := 0
	for _, want := range mustDenyTables {
		if !containsTable(tables, want) {

			t.Logf("threat-model table %q absent from this osqueryi build; skipping", want)
			continue
		}
		if !isSensitiveTable(want) {
			t.Errorf("table %q is threat-model-sensitive but NOT on the deny-list (sensitiveTables) — deny-list regressed/under-specified", want)
		}
		if _, err := q.QueryTable(ctx, want); err == nil || !strings.Contains(err.Error(), "not permitted") {
			t.Errorf("QueryTable(%q): want a 'not permitted' refusal before exec, got %v", want, err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("matches-zero: no threat-model table was present in the real .tables listing — the completeness check is vacuous (osquery build/parse problem?)")
	}
}

func TestRawSqlGated_Container(t *testing.T) {
	rows, err := realQuerier(t).QuerySQL(osqCtx(t), "SELECT count(*) AS n FROM shadow")
	if !errors.Is(err, ErrTableNotPermitted) || !strings.Contains(err.Error(), "not permitted") {
		t.Fatalf("raw SQL naming deny-listed `shadow` must be refused; got %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("refused raw SQL returned %d rows, want 0", len(rows))
	}
}

func TestInvalidTableName_Container(t *testing.T) {
	const bad = "os_version; DROP TABLE x"
	_, err := realQuerier(t).QueryTable(osqCtx(t), bad)
	if !errors.Is(err, ErrInvalidTableName) || !strings.Contains(err.Error(), "invalid table name") {
		t.Errorf("QueryTable(%q): want the 'invalid table name' shape refusal, got %v", bad, err)
	}
}

func containsTable(tables []string, want string) bool {
	for _, tbl := range tables {
		if tbl == want {
			return true
		}
	}
	return false
}

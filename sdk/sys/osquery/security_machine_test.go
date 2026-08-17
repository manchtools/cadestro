package osquery

import (
	"context"
	"errors"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

type osqueryPolicyAction int

const (
	osqueryAllowedTable osqueryPolicyAction = iota
	osqueryDeniedTable
	osqueryRawDeniedTable
	osqueryRawDeniedTableViaCTE
	osqueryRawProcessEnvSecret
)

type osqueryPolicyStep struct {
	name       string
	action     osqueryPolicyAction
	wantReject bool
}

// TestOSQueryPolicySecurityMachine models the osquery boundary as a policy
// automaton. Query input may reach the privileged osquery binary only if the
// resolved SQL cannot touch credential-bearing tables; every rejected state
// must fail with ErrTableNotPermitted before Runner execution.
func TestOSQueryPolicySecurityMachine(t *testing.T) {
	steps := []osqueryPolicyStep{
		{name: "allowed inventory table reaches osquery", action: osqueryAllowedTable},
		{name: "structured sensitive table is rejected", action: osqueryDeniedTable, wantReject: true},
		{name: "raw shadow table is rejected", action: osqueryRawDeniedTable, wantReject: true},
		{name: "raw CTE shadow table is rejected", action: osqueryRawDeniedTableViaCTE, wantReject: true},
		{name: "raw process environment table is rejected", action: osqueryRawProcessEnvSecret, wantReject: true},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			r := exectest.New(exec.Direct)
			if !step.wantReject {
				r.Push(exec.Result{Stdout: `[{"name":"linux"}]`}, nil)
			}
			c := &client{binaryPath: "/usr/bin/osqueryi", r: r}
			err := runOsqueryAction(c, step.action)
			if step.wantReject {
				if !errors.Is(err, ErrTableNotPermitted) {
					t.Fatalf("%s err = %v, want ErrTableNotPermitted", step.name, err)
				}
				if calls := r.Calls(); len(calls) != 0 {
					t.Fatalf("%s reached privileged osquery execution: %+v", step.name, calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s err = %v, want success", step.name, err)
			}
			if calls := r.Calls(); len(calls) != 1 || !calls[0].Escalate {
				t.Fatalf("%s calls = %+v, want one escalated osquery call", step.name, calls)
			}
		})
	}
}

func runOsqueryAction(c *client, action osqueryPolicyAction) error {
	ctx := context.Background()
	var err error
	switch action {
	case osqueryDeniedTable:
		_, err = c.QueryTable(ctx, "shadow")
	case osqueryRawDeniedTable:
		_, err = c.QuerySQL(ctx, "SELECT * FROM shadow")
	case osqueryRawDeniedTableViaCTE:
		_, err = c.QuerySQL(ctx, "WITH stolen AS (SELECT * FROM shadow) SELECT * FROM stolen")
	case osqueryRawProcessEnvSecret:
		_, err = c.QuerySQL(ctx, "SELECT * FROM process_envs WHERE key LIKE '%TOKEN%'")
	default: // osqueryAllowedTable
		_, err = c.QueryTable(ctx, "os_version")
	}
	return err
}

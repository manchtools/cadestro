package executor

import (
	"context"
	"testing"
)

func TestExecuteReboot_FailsClosedWithoutRunner(t *testing.T) {
	e := NewExecutor(nil)
	if e.runner != nil {
		t.Fatal("NewExecutor(nil) must leave the executor runner nil so reboot fails closed")
	}
	out, err := e.executeReboot(context.Background())
	if err == nil {
		t.Fatal("executeReboot with no privilege runner must fail closed, not schedule a real reboot")
	}
	if out != nil {
		t.Errorf("a refused reboot must not return command output, got %v", out)
	}
}

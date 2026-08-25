package idp

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	oidcDialControl = nil
	os.Exit(m.Run())
}

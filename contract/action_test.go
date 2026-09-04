package contract

import (
	"bytes"
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestActionDigestTracksDefinition(t *testing.T) {
	if _, err := ActionDigest(nil); err == nil {
		t.Fatal("nil action produced a digest")
	}
	action := &cadestrov1.Action{Id: &cadestrov1.ActionId{Value: "01K00000000000000000000001"}, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: "true"}}}
	first, err := ActionDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ActionDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || !bytes.Equal(first, second) {
		t.Fatalf("digest length/equality = %d/%v", len(first), bytes.Equal(first, second))
	}
	action.GetShell().Script = "false"
	changed, err := ActionDigest(action)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, changed) {
		t.Fatal("digest did not change with action definition")
	}
}

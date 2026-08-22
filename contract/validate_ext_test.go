package contract_test

import (
	"testing"

	"buf.build/go/protovalidate"

	pm "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestUlidOptional_AcceptsEmptyValidRejectsGarbage(t *testing.T) {
	t.Parallel()
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, pageToken string
		wantOK          bool
	}{
		{name: "empty", pageToken: "", wantOK: true},
		{name: "valid ulid", pageToken: "01HQ0000000000000000000000", wantOK: true},
		{name: "garbage", pageToken: "not-a-ulid", wantOK: false},
		{name: "wrong length", pageToken: "01HQ00000000000000000000", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Validate(&pm.ListUsersRequest{PageToken: tc.pageToken})
			if ok := err == nil; ok != tc.wantOK {
				t.Fatalf("ListUsersRequest{PageToken: %q} validate = %v (err: %v), want ok=%v", tc.pageToken, ok, err, tc.wantOK)
			}
		})
	}
}

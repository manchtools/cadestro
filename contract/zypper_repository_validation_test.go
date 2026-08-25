package contract_test

import (
	"testing"

	"buf.build/go/protovalidate"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestZypperRepositoryTypeValidation(t *testing.T) {
	t.Parallel()
	validator, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range definedEnumValues[cadestrov1.ZypperRepositoryType](t, cadestrov1.ZypperRepositoryType_name) {
		repository := &cadestrov1.ZypperRepository{Url: "https://mirror.example/repo", Type: typ}
		if err := validator.Validate(repository); err != nil {
			t.Errorf("defined type %d rejected: %v", typ, err)
		}
	}
	for _, typ := range undefinedEnumValues[cadestrov1.ZypperRepositoryType](t, cadestrov1.ZypperRepositoryType_name) {
		repository := &cadestrov1.ZypperRepository{Url: "https://mirror.example/repo", Type: typ}
		if err := validator.Validate(repository); err == nil {
			t.Errorf("undefined type %d accepted", typ)
		}
	}
}

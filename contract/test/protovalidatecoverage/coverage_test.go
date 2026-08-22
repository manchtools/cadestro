package main

import (
	"sort"
	"strings"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestEveryBoundableRequestFieldCarriesAValidateRule(t *testing.T) {
	var requestTypes int
	var missing []string

	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		md := mt.Descriptor()
		name := string(md.Name())
		if !strings.HasSuffix(name, "Request") {
			return true
		}
		requestTypes++

		messageCovered := proto.HasExtension(md.Options(), validate.E_Message)

		fields := md.Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if !boundable(fd) {
				continue
			}
			if proto.HasExtension(fd.Options(), validate.E_Field) {
				continue
			}
			if messageCovered {
				continue
			}
			missing = append(missing, name+"."+string(fd.Name())+" ("+fd.Kind().String()+")")
		}
		return true
	})

	if requestTypes == 0 {
		t.Fatal("no *Request message types discovered — the generated pm package did not register; the gate would pass vacuously")
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d bound-able request field(s) carry no buf.validate rule — add a constraint in the .proto and regenerate:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

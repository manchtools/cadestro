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

func boundable(fd protoreflect.FieldDescriptor) bool {
	switch {
	case fd.IsMap():
		return scalarValueKind(fd.MapValue().Kind())
	case fd.IsList():
		return scalarValueKind(fd.Kind())
	default:
		return boundableScalarKind(fd.Kind())
	}
}

func boundableScalarKind(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.StringKind, protoreflect.BytesKind,
		protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
		return true
	}
	return false
}

func scalarValueKind(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.MessageKind, protoreflect.GroupKind, protoreflect.EnumKind:
		return false
	}
	return true
}

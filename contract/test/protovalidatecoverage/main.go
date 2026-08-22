// Command protovalidatecoverage reports, per .proto file, how many fields
// across every message (requests and responses) carry a buf.validate rule.
//
// It is a reporting tool only — it always exits 0. The authoritative CI gate
// is the Go test TestEveryBoundableRequestFieldCarriesAValidateRule in
// coverage_test.go, which hard-fails when a bound-able *Request* field is
// missing a buf.validate rule and runs under the normal `go test` job. This
// binary stays useful for spotting uncovered RESPONSE fields (which the gate
// deliberately does not require) when triaging coverage by hand.
//
// Usage:
//
//	go run ./test/protovalidatecoverage
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type fileReport struct {
	path             string
	totalFields      int
	uncoveredFields  int
	uncoveredSamples []string
}

func main() {
	reports := map[string]*fileReport{}

	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		md := mt.Descriptor()
		path := string(md.ParentFile().Path())
		if !strings.HasPrefix(path, "cadestro/v1/") {
			return true
		}
		r := reports[path]
		if r == nil {
			r = &fileReport{path: path}
			reports[path] = r
		}
		messageCovered := proto.HasExtension(md.Options(), validate.E_Message)

		fields := md.Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if !boundable(fd) {
				continue
			}
			r.totalFields++
			if proto.HasExtension(fd.Options(), validate.E_Field) || messageCovered {
				continue
			}
			r.uncoveredFields++
			if len(r.uncoveredSamples) < 5 {
				r.uncoveredSamples = append(r.uncoveredSamples,
					fmt.Sprintf("  %s.%s (%s)", md.FullName(), fd.Name(), fd.Kind()))
			}
		}
		return true
	})

	if len(reports) == 0 {
		fmt.Fprintln(os.Stderr, "no messages registered — the generated pm package did not import")
		os.Exit(2)
	}

	var sorted []*fileReport
	for _, r := range reports {
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].uncoveredFields > sorted[j].uncoveredFields
	})

	var totalFields, totalUncovered int
	fmt.Println("== Proto buf.validate coverage ==")
	fmt.Printf("%-44s  %8s  %8s  %8s\n", "file", "fields", "covered", "missing")
	for _, r := range sorted {
		covered := r.totalFields - r.uncoveredFields
		fmt.Printf("%-44s  %8d  %8d  %8d\n", r.path, r.totalFields, covered, r.uncoveredFields)
		totalFields += r.totalFields
		totalUncovered += r.uncoveredFields
	}
	fmt.Println()
	fmt.Printf("TOTAL: %d fields, %d covered, %d missing.\n", totalFields, totalFields-totalUncovered, totalUncovered)

	if totalUncovered > 0 {
		fmt.Println()
		fmt.Println("== Sample of fields missing a buf.validate rule (first 5 per file) ==")
		for _, r := range sorted {
			if r.uncoveredFields == 0 {
				continue
			}
			fmt.Printf("\n# %s\n", r.path)
			for _, s := range r.uncoveredSamples {
				fmt.Println(s)
			}
			if r.uncoveredFields > len(r.uncoveredSamples) {
				fmt.Printf("... and %d more in this file\n", r.uncoveredFields-len(r.uncoveredSamples))
			}
		}
	}

	os.Exit(0)
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

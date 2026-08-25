package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const contractPackage = "cadestro.v1"

func main() {
	services := flag.String("services", "", "comma-separated service names to include (required)")
	invert := flag.Bool("invert", false, "emit every service EXCEPT those named")
	flag.Parse()

	if strings.TrimSpace(*services) == "" {
		fmt.Fprintln(os.Stderr, "rpcsurface: -services is required; a global list is not a valid "+
			"expectation for any single listener (see the package comment)")
		os.Exit(2)
	}
	want := map[string]bool{}
	for _, s := range strings.Split(*services, ",") {
		if s = strings.TrimSpace(s); s != "" {
			want[s] = true
		}
	}

	seen := map[string]bool{}
	var procedures []string
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if string(fd.Package()) != contractPackage {
			return true
		}
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			sd := svcs.Get(i)
			name := string(sd.Name())
			seen[name] = true
			if want[name] == *invert {
				continue
			}
			ms := sd.Methods()
			for j := 0; j < ms.Len(); j++ {
				procedures = append(procedures, fmt.Sprintf("/%s/%s", sd.FullName(), ms.Get(j).Name()))
			}
		}
		return true
	})

	if len(seen) == 0 {
		fmt.Fprintf(os.Stderr, "rpcsurface: no %s services in the descriptor registry — "+
			"the enumeration is broken, and emitting an empty set would let the gate pass vacuously\n", contractPackage)
		os.Exit(1)
	}

	for name := range want {
		if !seen[name] {
			fmt.Fprintf(os.Stderr, "rpcsurface: -services names %q but no such %s service exists — stale expectation\n", name, contractPackage)
			os.Exit(1)
		}
	}
	if len(procedures) == 0 {
		fmt.Fprintln(os.Stderr, "rpcsurface: selection matched zero procedures — refusing to emit an empty set")
		os.Exit(1)
	}

	sort.Strings(procedures)
	for _, p := range procedures {
		fmt.Println(p)
	}
}

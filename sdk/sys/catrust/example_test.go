package catrust_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/manchtools/cadestro/sdk/sys/catrust"
	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func ExampleNew() {

	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	m, err := catrust.New(catrust.CaCertificates, r)
	if err != nil {
		log.Fatal(err)
	}
	pem, err := os.ReadFile("/path/to/acme-root.pem")
	if err != nil {
		log.Fatal(err)
	}
	if err := m.Install(context.Background(), "acme-corp-root", pem); err != nil {
		log.Fatal(err)
	}
	anchors, err := m.List(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range anchors {
		fmt.Printf("%s: %s\n", a.Name, a.Subject)
	}
}

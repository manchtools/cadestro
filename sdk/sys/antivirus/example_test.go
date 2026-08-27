package antivirus_test

import (
	"context"
	"fmt"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/antivirus"
	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func ExampleNew() {
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	m, err := antivirus.New(antivirus.ClamAV, r)
	if err != nil {
		log.Fatal(err)
	}
	if err := m.UpdateSignatures(context.Background()); err != nil {
		log.Print(err)
	}
	res, err := m.Scan(context.Background(), "/home")
	if err != nil {
		log.Fatal(err)
	}
	for _, inf := range res.Infected {
		fmt.Printf("%s: %s\n", inf.File, inf.Signature)
	}
}

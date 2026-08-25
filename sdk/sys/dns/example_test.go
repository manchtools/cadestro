package dns_test

import (
	"context"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/dns"
	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func ExampleNew() {
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	m, err := dns.New(dns.Resolved, r)
	if err != nil {
		log.Fatal(err)
	}
	if err := m.Apply(context.Background(), dns.Config{
		Nameservers:   []string{"1.1.1.1", "9.9.9.9"},
		SearchDomains: []string{"corp.example"},
	}); err != nil {
		log.Fatal(err)
	}
}

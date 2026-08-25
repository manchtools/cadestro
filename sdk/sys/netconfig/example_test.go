package netconfig_test

import (
	"context"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/netconfig"
)

func ExampleNew() {
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	m, err := netconfig.New(netconfig.SystemdNetworkd, r)
	if err != nil {
		log.Fatal(err)
	}
	if err := m.Apply(context.Background(), netconfig.InterfaceConfig{
		Name:      "eth0",
		Mode:      netconfig.Static,
		Addresses: []string{"192.0.2.10/24"},
		Gateway:   "192.0.2.1",
		DNS:       []string{"1.1.1.1"},
		MTU:       1500,
		Routes:    []netconfig.Route{{Destination: "10.0.0.0/8", Gateway: "192.0.2.254"}},
	}); err != nil {
		log.Fatal(err)
	}
}

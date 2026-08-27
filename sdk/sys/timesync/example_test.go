package timesync_test

import (
	"context"
	"fmt"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/timesync"
)

func ExampleNew() {
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	m, err := timesync.New(timesync.Chrony, r)
	if err != nil {
		log.Fatal(err)
	}
	st, err := m.Status(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("synchronized=%v source=%s offset=%.6fs\n", st.Synchronized, st.Source, st.OffsetSeconds)
}

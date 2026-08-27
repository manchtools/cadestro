package desktop_test

import (
	"context"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/desktop"
	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func ExampleManager_ActiveSessions() {
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	m, err := desktop.New(r)
	if err != nil {
		log.Fatal(err)
	}

	sessions, err := m.ActiveSessions(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range sessions {
		ru, err := desktop.RunAsRunner(r, s)
		if err != nil {
			log.Print(err)
			continue
		}
		_ = ru
	}
}

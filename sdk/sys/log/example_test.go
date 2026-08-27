package log_test

import (
	"context"
	"fmt"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	syslog "github.com/manchtools/cadestro/sdk/sys/log"
)

func ExampleNew() {
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		log.Fatal(err)
	}
	s, err := syslog.New(syslog.Journald, r)
	if err != nil {
		log.Fatal(err)
	}
	lines, err := s.Query(context.Background(), syslog.Query{
		Unit:     "sshd.service",
		Priority: "warning",
		Lines:    200,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}

package terminal_test

import (
	"context"
	"io"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/terminal"
)

func ExampleManager_Open() {
	m, err := terminal.New()
	if err != nil {
		log.Fatal(err)
	}

	sess, err := m.Open(context.Background(), terminal.SessionConfig{
		User: "alice",
		Cols: 80,
		Rows: 24,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()

	if _, err := io.WriteString(sess, "echo hello\nexit\n"); err != nil {
		log.Fatal(err)
	}
	if _, err := sess.Wait(); err != nil {
		log.Print(err)
	}
}

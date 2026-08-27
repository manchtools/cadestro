package osquery_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/osquery"
)

func ExampleNew() {
	r, err := exec.NewRunner(exec.Sudo)
	if err != nil {
		log.Fatal(err)
	}

	q, err := osquery.New(r)
	if errors.Is(err, osquery.ErrNotInstalled) {

		return
	}
	if err != nil {
		log.Fatal(err)
	}

	rows, err := q.QueryTable(context.Background(), "os_version")
	if err != nil {
		log.Fatal(err)
	}
	for _, row := range rows {
		fmt.Println(row["name"])
	}
}

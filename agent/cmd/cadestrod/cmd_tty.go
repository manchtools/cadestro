package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/store"
)

func runTTY(args []string) int {
	fs := flag.NewFlagSet("tty", flag.ExitOnError)
	dataDir := fs.String("data-dir", credentials.DefaultDataDir, "Agent data directory")

	if len(args) == 0 {
		printTTYUsage()
		return 1
	}

	sub := args[0]

	_ = fs.Parse(args[1:])

	switch sub {
	case "-h", "--help", "help":
		printTTYUsage()
		return 0
	case "enable", "disable", "status":

	default:
		fmt.Fprintf(os.Stderr, "unknown tty subcommand: %s\n", sub)
		printTTYUsage()
		return 1
	}

	if sub == "enable" || sub == "disable" {
		if os.Geteuid() != 0 {
			fmt.Fprintf(os.Stderr, "Error: tty %s must be run as root (try: sudo cadestrod tty %s)\n", sub, sub)
			return 1
		}
	}

	st, err := store.OpenExisting(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: open agent store: %v\n", err)
		return 1
	}
	defer st.Close()
	ctx := context.Background()

	switch sub {
	case "enable":
		if err := st.SetTTYEnabled(ctx, true); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("TTY enabled.")
		return 0
	case "disable":
		if err := st.SetTTYEnabled(ctx, false); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("TTY disabled.")
		return 0
	case "status":
		enabled, err := st.IsTTYEnabled(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if enabled {
			fmt.Println("enabled")
			return 0
		}
		fmt.Println("disabled")
		return 1
	}
	return 1
}

func printTTYUsage() {
	fmt.Fprintln(os.Stderr, "usage: cadestrod tty {enable|disable|status} [--data-dir=PATH]")
}

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/manchtools/cadestro/agent/internal/luksd"

	"golang.org/x/term"
)

const luksTokenEnv = "CADESTRO_LUKS_TOKEN"

const maxLuksTokenBytes = 4096

const luksUsage = "usage: cadestrod luks set-passphrase [--token-file <path>|-]\n" +
	"       the token may also come from $" + luksTokenEnv + ", or be typed at the prompt"

func runLuks(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, luksUsage)
		os.Exit(1)
	}

	switch args[0] {
	case "set-passphrase":
		fs := flag.NewFlagSet("luks set-passphrase", flag.ExitOnError)
		tokenFile := fs.String("token-file", "",
			"File holding the one-time LUKS token, mode 0600 (\"-\" reads it from stdin)")
		fs.Parse(args[1:])

		token, err := resolveLuksToken(*tokenFile, os.Getenv(luksTokenEnv), os.Stdin, promptToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			fmt.Fprintln(os.Stderr, luksUsage)
			os.Exit(1)
		}

		runLuksSetPassphrase(token)
	default:
		fmt.Fprintf(os.Stderr, "unknown luks subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, luksUsage)
		os.Exit(1)
	}
}

func resolveLuksToken(tokenFile, envToken string, stdin io.Reader, prompt func() (string, error)) (string, error) {
	if tokenFile != "" {
		token, err := readLuksTokenFile(tokenFile, stdin)
		if err != nil {
			return "", err
		}
		if token == "" {
			return "", fmt.Errorf("token file %s is empty", tokenFile)
		}
		return token, nil
	}
	if token, err := normalizeLuksToken([]byte(envToken), luksTokenEnv); err != nil {
		return "", err
	} else if token != "" {
		return token, nil
	}
	if prompt != nil {
		token, err := prompt()
		if err != nil {
			return "", err
		}
		if token, err = normalizeLuksToken([]byte(token), "prompt"); err != nil {
			return "", err
		} else if token != "" {
			return token, nil
		}
	}
	return "", errors.New("no LUKS token supplied")
}

func readLuksTokenFile(path string, stdin io.Reader) (string, error) {
	if path == "-" {
		if stdin == nil {
			return "", errors.New("--token-file - was given but stdin is unavailable")
		}
		line, err := bufio.NewReader(io.LimitReader(stdin, maxLuksTokenBytes+1)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read token from stdin: %w", err)
		}
		return normalizeLuksToken([]byte(line), "stdin")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("token file %s is mode %04o; it must not be readable beyond its owner (chmod 600 %s)", path, perm, path)
	}
	b, err := io.ReadAll(io.LimitReader(file, maxLuksTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	return normalizeLuksToken(b, path)
}

func normalizeLuksToken(raw []byte, source string) (string, error) {
	if len(raw) > maxLuksTokenBytes {
		return "", fmt.Errorf("LUKS token from %s exceeds %d bytes", source, maxLuksTokenBytes)
	}
	return strings.TrimSpace(string(raw)), nil
}

func promptToken() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", nil
	}
	fmt.Print("Enter the one-time LUKS token: ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func runLuksURI(rawURI string) {

	if !strings.HasPrefix(rawURI, "cadestro://") {
		fmt.Fprintf(os.Stderr, "error: not a cadestro:// URI\n")
		os.Exit(1)
	}
	normalizedURI := "https://" + strings.TrimPrefix(rawURI, "cadestro://")
	parsed, err := url.Parse(normalizedURI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid URI: %v\n", err)
		os.Exit(1)
	}

	token := parsed.Query().Get("token")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: token parameter is required in URI")
		os.Exit(1)
	}

	runLuksSetPassphrase(token)

	fmt.Println("\nPress Enter to close...")
	fmt.Scanln()
	os.Exit(0)
}

func runLuksSetPassphrase(token string) {
	client := luksd.NewClient(luksd.DefaultSocketPath)
	if err := client.SetPassphrase(token, promptPassphrase); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("LUKS passphrase set successfully.")
}

func promptPassphrase() (string, error) {
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		remaining := maxAttempts - attempt

		fmt.Print("Enter LUKS passphrase: ")
		pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read passphrase: %w", err)
		}

		fmt.Print("Confirm passphrase: ")
		pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read confirmation: %w", err)
		}

		if string(pw1) != string(pw2) {
			if remaining > 0 {
				fmt.Printf("Passphrases do not match. %d attempt(s) remaining.\n", remaining)
			}
			continue
		}

		candidate := string(pw1)

		if len(candidate) < 16 {
			if remaining > 0 {
				fmt.Printf("Passphrase must be at least 16 characters. %d attempt(s) remaining.\n", remaining)
			}
			continue
		}
		return candidate, nil
	}

	fmt.Fprintln(os.Stderr, "Too many failed attempts.")
	return "", nil
}

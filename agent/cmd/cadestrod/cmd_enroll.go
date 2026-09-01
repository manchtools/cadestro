package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/deviceauth"
)

func parseRegistrationURI(rawURI string) (*registrationURI, error) {

	normalizedURI := strings.Replace(rawURI, "cadestro://", "https://", 1)

	parsed, err := url.Parse(normalizedURI)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	token := parsed.Query().Get("token")
	if token == "" {
		return nil, fmt.Errorf("token parameter is required in URI")
	}
	pin := strings.TrimSpace(parsed.Query().Get("pin"))
	if pin == "" {
		return nil, fmt.Errorf("pin parameter is required in URI")
	}

	return &registrationURI{
		ServerURL: fmt.Sprintf("https://%s", parsed.Host),
		Token:     token,

		Pin: pin,
	}, nil
}

func resolveEnrollToken(flagToken, tokenFile, envToken string) (string, error) {
	if tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("read token file %s: %w", tokenFile, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	if envToken != "" {
		return strings.TrimSpace(envToken), nil
	}
	if flagToken != "" {
		fmt.Fprintln(os.Stderr, "warning: passing -token on the command line is insecure (visible in /proc/<pid>/cmdline); prefer -token-file or the CADESTRO_REGISTRATION_TOKEN environment variable")
		return strings.TrimSpace(flagToken), nil
	}
	return "", nil
}

func runEnroll(args []string, euid int) int {
	if euid != 0 {
		fmt.Fprintln(os.Stderr, "error: enroll must run as root")
		return 1
	}
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	token := fs.String("token", "", "Registration token (INSECURE on argv; prefer -token-file or CADESTRO_REGISTRATION_TOKEN)")
	tokenFile := fs.String("token-file", "", "Path to a file containing the registration token (preferred over -token)")
	server := fs.String("server", "", "Control server URL")
	pin := fs.String("pin", "", "Required CA fingerprint pin (SHA-256 hex of the control CA)")
	dataDir := fs.String("data-dir", credentials.DefaultDataDir, "Data directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if value := os.Getenv("CADESTRO_DATA_DIR"); value != "" {
		*dataDir = value
	}

	caPin := *pin
	fromURI := false

	if fs.NArg() > 0 {
		arg := fs.Arg(0)
		if strings.HasPrefix(arg, "cadestro://") {
			parsed, err := parseRegistrationURI(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
			*server = parsed.ServerURL
			*token = parsed.Token
			if parsed.Pin != "" {
				caPin = parsed.Pin
			}
			fromURI = true
		}
	}

	resolvedToken := *token
	if !fromURI {
		rt, err := resolveEnrollToken(*token, *tokenFile, os.Getenv("CADESTRO_REGISTRATION_TOKEN"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		resolvedToken = rt
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store := credentials.NewStore(*dataDir)
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read hostname: %v\n", err)
		return 1
	}
	result, err := deviceauth.Enroll(ctx, deviceauth.EnrollmentRequest{
		ServerURL: *server,
		Token:     resolvedToken,
		CAPin:     caPin,
		Hostname:  hostname,
		Version:   version,
	}, store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: enrollment failed: %v\n", err)
		return 1
	}
	if result.AlreadyEnrolled {
		fmt.Printf("Agent is already enrolled (device ID: %s)\n", result.Credentials.DeviceID)
		return 0
	}
	fmt.Printf("Enrolled successfully. Device ID: %s\n", result.Credentials.DeviceID)
	return 0
}

func registrationURIRefusedByHandler(uri string) bool {
	return strings.HasPrefix(uri, "cadestro://")
}

type registrationURI struct {
	ServerURL string
	Token     string
	Pin       string
}

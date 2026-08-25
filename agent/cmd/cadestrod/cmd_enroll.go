package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/agent/internal/deviceauth"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
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

func runEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	token := fs.String("token", "", "Registration token (INSECURE on argv; prefer -token-file or CADESTRO_REGISTRATION_TOKEN)")
	tokenFile := fs.String("token-file", "", "Path to a file containing the registration token (preferred over -token)")
	server := fs.String("server", "", "Control server URL")
	pin := fs.String("pin", "", "Required CA fingerprint pin (SHA-256 hex of the control CA)")
	socketPath := fs.String("socket", deviceauth.EnrollSocketPath, "Agent enrollment socket")
	fs.Parse(args)

	caPin := *pin
	fromURI := false

	if fs.NArg() > 0 {
		arg := fs.Arg(0)
		if strings.HasPrefix(arg, "cadestro://") {
			parsed, err := parseRegistrationURI(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
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
			os.Exit(1)
		}
		resolvedToken = rt
	}

	if resolvedToken == "" || *server == "" || strings.TrimSpace(caPin) == "" {
		fmt.Fprintln(os.Stderr, "error: a control server URL, registration token, and CA fingerprint pin are required")
		fmt.Fprintln(os.Stderr, "usage: cadestrod enroll -server=URL -token-file=PATH -pin=SHA256")
		fmt.Fprintln(os.Stderr, "   or: CADESTRO_REGISTRATION_TOKEN=… cadestrod enroll -server=URL -pin=SHA256")
		fmt.Fprintln(os.Stderr, "   or: cadestrod enroll 'cadestro://server:port?token=xxx&pin=…'")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	httpClient := unixSocketHTTPClient(*socketPath)
	client := cadestrov1connect.NewDeviceAuthServiceClient(httpClient, "http://localhost")

	status, err := client.GetEnrollmentStatus(ctx, connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot connect to agent enrollment socket at %s\n", *socketPath)
		fmt.Fprintln(os.Stderr, "Is the agent service running? Check: systemctl status cadestrod")
		os.Exit(1)
	}

	if status.Msg.Enrolled {
		fmt.Printf("Agent is already enrolled (device ID: %s)\n", status.Msg.GetDeviceId().GetValue())
		return
	}

	resp, err := client.Enroll(ctx, connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl:        *server,
		Token:            resolvedToken,
		CaFingerprintPin: strings.ReplaceAll(strings.TrimSpace(caPin), ":", ""),
	}))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: enrollment failed: %v\n", err)
		os.Exit(1)
	}

	if !resp.Msg.Success {
		fmt.Fprintf(os.Stderr, "error: enrollment failed: %s\n", resp.Msg.Error)
		os.Exit(1)
	}

	fmt.Printf("Enrolled successfully. Device ID: %s\n", resp.Msg.GetDeviceId().GetValue())
}

func registrationURIRefusedByHandler(uri string) bool {
	return strings.HasPrefix(uri, "cadestro://") && !strings.HasPrefix(uri, "cadestro://luks/")
}

type registrationURI struct {
	ServerURL string
	Token     string
	Pin       string
}

func unixSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

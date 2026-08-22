package luksd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

type PassphraseReader func() (string, error)

type Client struct {
	socketPath string
	dialer     func() (net.Conn, error)
	now        func() time.Time
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	c := &Client{socketPath: socketPath, now: time.Now}
	c.dialer = func() (net.Conn, error) {
		return net.DialTimeout("unix", c.socketPath, 5*time.Second)
	}
	return c
}

func (c *Client) SetPassphrase(token string, read PassphraseReader) error {
	if token == "" {
		return errors.New("token is required")
	}
	passphrase, err := read()
	if err != nil {
		return err
	}
	if passphrase == "" {
		return errors.New("no passphrase provided")
	}

	conn, err := c.dialer()
	if err != nil {
		return fmt.Errorf("connect to LUKS daemon at %s: %w (is the agent running?)", c.socketPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(c.now().Add(60 * time.Second))

	if err := json.NewEncoder(conn).Encode(Request{Token: token, Passphrase: passphrase}); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if !resp.OK {
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		return fmt.Errorf("LUKS daemon rejected the request (%s)", resp.Code)
	}
	return nil
}

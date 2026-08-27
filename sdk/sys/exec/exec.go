package exec

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-cmd/cmd"
)

var killGrace = 5 * time.Second

func awaitStatusOrKill(c *cmd.Cmd, statusChan <-chan cmd.Status) cmd.Status {
	_ = c.Stop()

	term := time.NewTimer(killGrace)
	defer term.Stop()
	select {
	case status := <-statusChan:
		return status
	case <-term.C:
	}

	if pid := c.Status().PID; pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}

	kill := time.NewTimer(killGrace)
	defer kill.Stop()
	select {
	case status := <-statusChan:
		return status
	case <-kill.C:

		return c.Status()
	}
}

// ValidateCommandEnv checks a Command.Env slice with the EXACT rules the real
// Runner enforces before it spawns any child: each entry must be KEY=VALUE
// (ErrInvalidEnvVar), the key must not be a hijack-prone name on the blocklist
// (ErrBlockedEnvVar — PATH/LD_PRELOAD/BASH_ENV/…), and the key must not be a
// name the Runner reserves for deterministic output (ErrReservedEnvVar —
// LC_ALL/LANG/NO_COLOR). It is exported so exectest.FakeRunner can apply the
// identical gate, keeping the unit tier faithful to the real env boundary — an
// adversarial Command.Env is rejected the same way against a fake as against a
// real Runner.
func ValidateCommandEnv(env []string) error {
	for _, e := range env {
		key, _, ok := strings.Cut(e, "=")
		if !ok {
			return fmt.Errorf("%w: env entry must be KEY=VALUE, got %q", ErrInvalidEnvVar, e)
		}
		if !IsAllowedEnvVar(key) {
			return fmt.Errorf("%w: refusing to forward env var %q to child (hijack-prone names like LD_PRELOAD, PATH, BASH_ENV are refused at this boundary)", ErrBlockedEnvVar, key)
		}
	}
	for _, e := range env {
		if key, _, _ := strings.Cut(e, "="); isReservedEnvVar(key) {
			return fmt.Errorf("%w: %q is forced by the Runner (LC_ALL=C/LANG=C/NO_COLOR=1) and may not be set via Command.Env", ErrReservedEnvVar, key)
		}
	}
	return nil
}

func composeEnv(childPath string, envVars []string) []string {
	env := make([]string, 0, len(envVars)+1)
	if childPath != "" {
		env = append(env, "PATH="+childPath)
	}
	return append(env, envVars...)
}

func runStreamingWithStdin(ctx context.Context, name string, args []string, stdin io.Reader, env []string, dir string, callback OutputCallback) (*Result, error) {
	c := cmd.NewCmdOptions(cmd.Options{
		Buffered:       false,
		Streaming:      true,
		LineBufferSize: 4 * MaxOutputBytes,
	}, name, args...)

	if dir != "" {
		c.Dir = dir
	}
	if env != nil {
		c.Env = env
	}

	var statusChan <-chan cmd.Status
	if stdin != nil {
		statusChan = c.StartWithStdin(stdin)
	} else {
		statusChan = c.Start()
	}

	var stdoutSeq, stderrSeq int64
	var stdoutBuf, stderrBuf strings.Builder
	var stdoutBytes, stderrBytes int64

	recordLine := func(stream StreamType, line string) {
		lineBytes := int64(len(line) + 1)
		if stream == StreamStdout {
			if atomic.AddInt64(&stdoutBytes, lineBytes) <= int64(MaxOutputBytes) {
				stdoutBuf.WriteString(line + "\n")
			}
			if callback != nil {
				callback(StreamStdout, line+"\n", atomic.AddInt64(&stdoutSeq, 1)-1)
			}
		} else {
			if atomic.AddInt64(&stderrBytes, lineBytes) <= int64(MaxOutputBytes) {
				stderrBuf.WriteString(line + "\n")
			}
			if callback != nil {
				callback(StreamStderr, line+"\n", atomic.AddInt64(&stderrSeq, 1)-1)
			}
		}
	}

	drainRemaining := func(ch <-chan string, stream StreamType) {
		for line := range ch {
			recordLine(stream, line)
		}
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case line, ok := <-c.Stdout:
				if !ok {
					drainRemaining(c.Stderr, StreamStderr)
					return
				}
				recordLine(StreamStdout, line)
			case line, ok := <-c.Stderr:
				if !ok {
					drainRemaining(c.Stdout, StreamStdout)
					return
				}
				recordLine(StreamStderr, line)
			case <-ctx.Done():

				return
			}
		}
	}()

	var (
		status cmd.Status
		runErr error
	)
	select {
	case status = <-statusChan:
		runErr = status.Error
	case <-ctx.Done():
		status = awaitStatusOrKill(c, statusChan)
		runErr = ctx.Err()
	}
	<-done

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()
	if atomic.LoadInt64(&stdoutBytes) > int64(MaxOutputBytes) {
		stdoutStr += "\n[output truncated]"
	}
	if atomic.LoadInt64(&stderrBytes) > int64(MaxOutputBytes) {
		stderrStr += "\n[output truncated]"
	}

	return &Result{
		ExitCode: status.Exit,
		Stdout:   stdoutStr,
		Stderr:   stderrStr,
	}, runErr
}

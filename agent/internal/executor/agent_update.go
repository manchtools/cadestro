package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	sdk "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
	"github.com/manchtools/cadestro/sdk/sys/remote"
)

type AgentUpdateConfig struct {
	Version    string
	DataDir    string
	BinaryPath string
	Shutdown   func()
}

func (e *Executor) ResetUpdateCycle() {
	e.agentUpdateExecutedMu.Lock()
	e.agentUpdateExecuted = false
	e.agentUpdateExecutedMu.Unlock()
}

func (e *Executor) markAgentUpdateExecuted() bool {
	e.agentUpdateExecutedMu.Lock()
	defer e.agentUpdateExecutedMu.Unlock()
	if e.agentUpdateExecuted {
		return false
	}
	e.agentUpdateExecuted = true
	return true
}

func (e *Executor) executeAgentUpdate(ctx context.Context, params *pb.AgentUpdateParams) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	cfg := e.updateCfg
	if cfg == nil {
		return nil, false, fmt.Errorf("agent update not configured")
	}

	if !e.markAgentUpdateExecuted() {
		e.logger.Warn("skipping duplicate AGENT_UPDATE action in this sync cycle")
		return &pb.CommandOutput{Stdout: "Skipped: another AGENT_UPDATE already executed this cycle"}, false, nil
	}

	arch := getArchEntry(params)
	if arch == nil {
		e.logger.Info("no agent update entry for this architecture", "arch", runtime.GOARCH)
		return &pb.CommandOutput{Stdout: fmt.Sprintf("No update entry for architecture %s", runtime.GOARCH)}, false, nil
	}

	if err := sdk.ValidateHTTPSURL(arch.BinaryUrl); err != nil {
		return nil, false, fmt.Errorf("binary URL validation: %w", err)
	}

	if arch.ChecksumUrl == "" {
		return nil, false, fmt.Errorf("agent update rejected: checksum_url is required")
	}
	if err := sdk.ValidateHTTPSURL(arch.ChecksumUrl); err != nil {
		return nil, false, fmt.Errorf("checksum URL validation: %w", err)
	}
	expectedChecksum, err := downloadAndExtractChecksum(ctx, arch.ChecksumUrl, extractFilename(arch.BinaryUrl), updateRedirectPolicy(params))
	if err != nil {
		return nil, false, fmt.Errorf("download checksum: %w", err)
	}

	updateDir := filepath.Join(cfg.DataDir, "update")
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		return nil, false, fmt.Errorf("create update dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(updateDir, "agent-update-*.tmp")
	if err != nil {
		return nil, false, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := fetchArtifact(ctx, arch.BinaryUrl, tmpPath, expectedChecksum, "0755", updateRedirectPolicy(params)); err != nil {
		if errors.Is(err, remote.ErrIntegrity) {
			return nil, false, fmt.Errorf("binary does not match the signed release manifest: %w", err)
		}
		return nil, false, fmt.Errorf("download binary: %w", err)
	}

	newVersion, err := e.getBinaryVersion(tmpPath)
	if err != nil {
		return nil, false, fmt.Errorf("version check on downloaded binary: %w", err)
	}

	if newVersion == cfg.Version {
		e.logger.Info("agent is already at the latest version", "version", cfg.Version)
		return &pb.CommandOutput{Stdout: fmt.Sprintf("Already at version %s", cfg.Version)}, false, nil
	}

	if !params.AllowDowngrade {
		cmp, cmpErr := compareAgentVersion(cfg.Version, newVersion)
		if cmpErr != nil {
			return nil, false, fmt.Errorf("refusing update: cannot compare versions (running %q, candidate %q): %w", cfg.Version, newVersion, cmpErr)
		}
		if cmp > 0 {
			return nil, false, fmt.Errorf("refusing downgrade: candidate %s is older than running %s (set allow_downgrade on the action to override)", newVersion, cfg.Version)
		}
		if cmp == 0 {

			e.logger.Info("agent is already at an equivalent version", "running", cfg.Version, "candidate", newVersion)
			return &pb.CommandOutput{Stdout: fmt.Sprintf("Already at version %s", cfg.Version)}, false, nil
		}
	}

	e.logger.Info("updating agent", "from", cfg.Version, "to", newVersion)

	selfTestCtx, selfTestCancel := context.WithTimeout(ctx, 60*time.Second)
	defer selfTestCancel()

	e.logger.Info("running self-test on new binary", "path", tmpPath)
	selfTestResult, selfTestErr := e.runnerOrDirect().Run(selfTestCtx, sysexec.Command{
		Name: tmpPath,
		Args: []string{"self-test", "--data-dir=" + cfg.DataDir, "--timeout=55s"},
	})

	if selfTestErr != nil || selfTestResult.ExitCode != 0 {
		if selfTestErr == nil {
			selfTestErr = fmt.Errorf("self-test exited with code %d", selfTestResult.ExitCode)
		}
		combined := strings.TrimSpace(selfTestResult.Stdout + "\n" + selfTestResult.Stderr)
		e.logger.Error("self-test failed, keeping current binary",
			"error", selfTestErr,
			"output", combined)
		out := &pb.CommandOutput{
			Stdout: fmt.Sprintf("Self-test failed for version %s: %v", newVersion, selfTestErr),
			Stderr: combined,
		}
		return out, false, fmt.Errorf("self-test failed: %w", selfTestErr)
	}
	e.logger.Info("self-test passed", "output", selfTestResult.Stdout)

	bakPath := cfg.BinaryPath + ".bak"
	newBinary, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, false, fmt.Errorf("read staged binary %s: %w", tmpPath, err)
	}

	if err := e.deps.fs.WriteFile(ctx, cfg.BinaryPath, newBinary, sysfs.WriteOptions{Mode: 0o755, Backup: bakPath}); err != nil {
		return nil, false, fmt.Errorf("swap binary at %s: %w", cfg.BinaryPath, err)
	}

	unitCtx, unitCancel := context.WithTimeout(ctx, 30*time.Second)
	unitRes, unitErr := e.runnerOrDirect().Run(unitCtx, sysexec.Command{
		Name: cfg.BinaryPath,
		Args: []string{"install-unit", "--data-dir=" + cfg.DataDir},
	})
	unitCancel()
	if unitErr != nil || unitRes.ExitCode != 0 {
		e.logger.Error("install-unit on new binary failed; the startup reconcile will retry after respawn",
			"error", unitErr, "exit_code", unitRes.ExitCode,
			"stderr", strings.TrimSpace(unitRes.Stderr))
	}

	stdout := fmt.Sprintf("Updated from %s to %s. Restarting.", cfg.Version, newVersion)
	e.logger.Info(stdout)

	if cfg.Shutdown != nil {
		go func() {
			time.Sleep(3 * time.Second)
			cfg.Shutdown()
		}()
	}

	return &pb.CommandOutput{Stdout: stdout}, true, nil
}

type agentVersion struct {
	core   [3]int
	pre    bool
	preNum int
	preRaw string
}

func compareAgentVersion(a, b string) (int, error) {
	pa, err := parseAgentVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseAgentVersion(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < len(pa.core); i++ {
		if pa.core[i] != pb.core[i] {
			if pa.core[i] < pb.core[i] {
				return -1, nil
			}
			return 1, nil
		}
	}

	if pa.pre != pb.pre {
		if pa.pre {
			return -1, nil
		}
		return 1, nil
	}
	if !pa.pre {
		return 0, nil
	}

	if pa.preNum != pb.preNum {
		if pa.preNum < pb.preNum {
			return -1, nil
		}
		return 1, nil
	}
	if pa.preRaw < pb.preRaw {
		return -1, nil
	}
	if pa.preRaw > pb.preRaw {
		return 1, nil
	}
	return 0, nil
}

func parseAgentVersion(v string) (agentVersion, error) {
	var out agentVersion
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return out, fmt.Errorf("empty version")
	}
	core := v
	if i := strings.IndexByte(v, '-'); i >= 0 {
		core = v[:i]
		suffix := v[i+1:]
		if suffix == "" {
			return out, fmt.Errorf("invalid version %q: empty pre-release suffix", v)
		}
		out.pre = true
		out.preRaw = suffix
		if n, ok := parseRCNumber(suffix); ok {
			out.preNum = n
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return out, fmt.Errorf("invalid version %q: want vYYYY.MM[.PP][-rcN]", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("invalid version component %q in %q: %w", p, v, err)
		}
		out.core[i] = n
	}
	return out, nil
}

func parseRCNumber(suffix string) (int, bool) {
	if !strings.HasPrefix(suffix, "rc") {
		return 0, false
	}
	n, err := strconv.Atoi(suffix[2:])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func extractFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return filepath.Base(rawURL)
	}
	return filepath.Base(u.Path)
}

func updateRedirectPolicy(params *pb.AgentUpdateParams) remote.RedirectPolicy {
	if params.GetAllowRedirect() {
		return remote.RedirectCrossOrigin
	}
	return remote.RedirectSameOrigin
}

func downloadAndExtractChecksum(ctx context.Context, checksumURL, filename string, redirect remote.RedirectPolicy) (string, error) {

	body, err := remote.FetchBytes(ctx, remote.HTTPConfig{
		URL:      checksumURL,
		Redirect: redirect,
		Client:   remoteHTTPClient,
	})
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	signatureURL, err := releaseSignatureURL(checksumURL)
	if err != nil {
		return "", err
	}
	signature, err := remote.FetchBytes(ctx, remote.HTTPConfig{
		URL:      signatureURL,
		Redirect: redirect,
		Client:   remoteHTTPClient,
	})
	if err != nil {
		return "", fmt.Errorf("download release signature: %w", err)
	}
	if err := verifyReleaseManifest(body, signature); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		checksumHex := parts[0]
		name := strings.TrimPrefix(strings.TrimPrefix(parts[1], "./"), "*")
		if name == filename {
			if len(checksumHex) != 64 {
				return "", fmt.Errorf("invalid checksum length for %s: %d", filename, len(checksumHex))
			}
			return strings.ToLower(checksumHex), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksum file: %w", err)
	}
	return "", fmt.Errorf("checksum for %q not found in checksum file", filename)
}

func getArchEntry(params *pb.AgentUpdateParams) *pb.AgentUpdateArch {
	switch runtime.GOARCH {
	case "amd64":
		return params.Amd64
	case "arm64":
		return params.Arm64
	default:
		return nil
	}
}

func (e *Executor) getBinaryVersion(binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := e.runnerOrDirect().Run(ctx, sysexec.Command{Name: binaryPath, Args: []string{"version"}})
	if err != nil {
		return "", fmt.Errorf("run %s version: %w", binaryPath, err)
	}
	v := strings.TrimSpace(result.Stdout)
	if v == "" {
		return "", fmt.Errorf("binary returned empty version")
	}

	parts := strings.Fields(v)
	if len(parts) >= 2 {
		v = parts[1]
	}
	return v, nil
}

func readUpdateState(dataDir string) (phase, version string, err error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "update", "state.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", err
	}

	type state struct {
		Phase   string `json:"phase"`
		Version string `json:"version"`
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", err
	}
	return s.Phase, s.Version, nil
}

func clearUpdateState(dataDir string) {
	os.Remove(filepath.Join(dataDir, "update", "state.json"))
}

func CheckStartupUpdateState(dataDir string, logger interface {
	Info(string, ...any)
	Warn(string, ...any)
}, now func() time.Time) {
	phase, _, err := readUpdateState(dataDir)
	if err != nil {
		logger.Warn("failed to read update state", "error", err)
		return
	}
	if phase != "" {
		logger.Info("cleaning up stale update state", "phase", phase)
		clearUpdateState(dataDir)
	}

	updateDir := filepath.Join(dataDir, "update")
	entries, err := os.ReadDir(updateDir)
	if err != nil {

		if !os.IsNotExist(err) {
			logger.Warn("failed to read update dir for stale tmp sweep",
				"dir", updateDir, "error", err)
		}
		return
	}
	cutoff := now().Add(-24 * time.Hour)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "agent-update-") || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(updateDir, name)
		if err := os.Remove(path); err != nil {
			logger.Warn("failed to remove stale update tmp file", "path", path, "error", err)
			continue
		}
		logger.Info("removed stale update tmp file", "path", path, "age", now().Sub(info.ModTime()).Round(time.Second))
	}
}

package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const testTerminalAdminGroup = "cadestro-sudo-test"

func TestGenerateTerminalAdminLimitedSudoConfig_GroupInterpolation(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	assert.Contains(t, out, "%"+testTerminalAdminGroup+" ALL=",
		"the passed group name must appear in every rule")
}

func TestGenerateTerminalAdminLimitedSudoConfig_NOPASSWD(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if !strings.HasPrefix(trimmed, "%"+testTerminalAdminGroup) {
			continue
		}

		runspec := strings.SplitN(trimmed, "ALL=(ALL)", 2)
		if len(runspec) != 2 {
			t.Fatalf("unexpected rule shape: %q", line)
		}
		body := strings.TrimSpace(runspec[1])
		if strings.HasPrefix(body, "!") {
			continue
		}
		assert.True(t, strings.HasPrefix(body, "NOPASSWD:"),
			"affirmative grant must start with NOPASSWD: — %q", line)
	}
}

func TestGenerateTerminalAdminLimitedSudoConfig_DefaultsBlock(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	g := "%" + testTerminalAdminGroup
	for _, want := range []string{
		"Defaults:" + g + " requiretty",
		"Defaults:" + g + " env_reset",
		"Defaults:" + g + " !lecture",
		"Defaults:" + g + " timestamp_timeout=0",
	} {
		assert.Contains(t, out, want,
			"ADR T4: group-scoped Defaults block must include %q", want)
	}
}

func TestGenerateTerminalAdminLimitedSudoConfig_DeniesEditors(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)

	editors := []string{
		"/usr/bin/vim", "/usr/bin/vi", "/usr/bin/vimdiff", "/usr/bin/view", "/usr/bin/nvim",
		"/usr/bin/emacs", "/usr/bin/emacsclient",
		"/usr/bin/nano", "/bin/nano",
		"/usr/bin/less", "/usr/bin/more", "/usr/bin/most",
		"/usr/bin/ed", "/usr/bin/ex",
		"/usr/bin/mc", "/usr/bin/joe", "/usr/bin/jed",
	}
	for _, editor := range editors {
		assert.Contains(t, out, "!"+editor,
			"ADR T2: editor %s must appear in the deny block", editor)
	}
}

func TestGenerateTerminalAdminLimitedSudoConfig_DeniesShells(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	shells := []string{
		"/bin/sh", "/bin/bash", "/bin/dash", "/bin/zsh", "/bin/ksh", "/bin/csh", "/bin/tcsh", "/bin/fish",
		"/usr/bin/sh", "/usr/bin/bash", "/usr/bin/dash", "/usr/bin/zsh", "/usr/bin/ksh", "/usr/bin/csh", "/usr/bin/tcsh", "/usr/bin/fish",
		"/usr/bin/env",
	}
	for _, shell := range shells {
		assert.Contains(t, out, "!"+shell,
			"ADR T3: shell %s must appear in the deny block", shell)
	}
}

func TestGenerateTerminalAdminLimitedSudoConfig_DeniesPersistenceVectors(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	vectors := []string{
		"/usr/bin/at", "/usr/bin/atq", "/usr/bin/atrm", "/usr/bin/batch",
		"/usr/bin/crontab",
		"/usr/sbin/dpkg-divert", "/usr/bin/dpkg-divert",
		"/usr/bin/update-alternatives", "/usr/sbin/update-alternatives",
	}
	for _, vector := range vectors {
		assert.Contains(t, out, "!"+vector,
			"ADR T5: persistence vector %s must appear in the deny block", vector)
	}
}

func TestGenerateTerminalAdminLimitedSudoConfig_AgentProtectionIsRealDeny(t *testing.T) {
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	assert.NotContains(t, out, "!!",
		"double-bang in sudoers is an even negation = ALLOW; agent/visudo protection must use a single '!' deny")
	assert.Contains(t, out, "!/usr/bin/systemctl * cadestrod*",
		"LIMITED template must deny controlling the cadestrod unit")
	assert.Contains(t, out, "!/usr/bin/visudo",
		"LIMITED template must deny visudo (sudoers edit is root escalation)")
	assert.Contains(t, out, "!/usr/sbin/visudo",
		"LIMITED template must deny the /usr/sbin/visudo path too")
}

func TestGenerateLimitedSudoConfig_AgentProtectionIsRealDeny(t *testing.T) {
	out := generateLimitedSudoConfig(testTerminalAdminGroup)
	assert.NotContains(t, out, "!!",
		"double-bang grants the command it claims to deny — legacy LIMITED template must use a single '!'")
}

func TestTerminalAdminDefaults_ScopedToGroup_NotHostGlobal(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
	}{
		{"limited", generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)},
		{"full", generateTerminalAdminFullSudoConfig(testTerminalAdminGroup)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, setting := range []string{"requiretty", "env_reset", "!lecture", "timestamp_timeout=0"} {
				assert.NotContains(t, tc.out, "Defaults "+setting,
					"host-global Defaults leaks onto every sudo on the box; scope it to the group")
				assert.Contains(t, tc.out, "Defaults:%"+testTerminalAdminGroup+" "+setting,
					"Defaults must be scoped to the TerminalAdmin group")
			}
		})
	}
}

func TestGenerateTerminalAdminFullSudoConfig_GroupInterpolation(t *testing.T) {
	out := generateTerminalAdminFullSudoConfig(testTerminalAdminGroup)
	assert.Contains(t, out, "%"+testTerminalAdminGroup,
		"the passed group name must appear in the rule")
}

func TestGenerateTerminalAdminFullSudoConfig_NOPASSWD_ALL(t *testing.T) {
	out := generateTerminalAdminFullSudoConfig(testTerminalAdminGroup)
	assert.Contains(t, out, "%"+testTerminalAdminGroup+" ALL=(ALL:ALL) NOPASSWD: ALL",
		"FULL template must grant ALL=(ALL:ALL) NOPASSWD: ALL")
}

func TestGenerateTerminalAdminFullSudoConfig_DefaultsBlock(t *testing.T) {
	out := generateTerminalAdminFullSudoConfig(testTerminalAdminGroup)
	g := "%" + testTerminalAdminGroup
	for _, want := range []string{
		"Defaults:" + g + " requiretty",
		"Defaults:" + g + " env_reset",
		"Defaults:" + g + " !lecture",
		"Defaults:" + g + " timestamp_timeout=0",
	} {
		assert.Contains(t, out, want,
			"ADR T4: group-scoped Defaults block must apply to FULL as well — missing %q", want)
	}
}

func TestSetupSudoPolicy_RoutesTerminalAdminLimitedEnumToNewGenerator(t *testing.T) {
	out := contentForAccessLevel(t, pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED)
	want := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	assert.Equal(t, want, out,
		"AccessLevel=TERMINAL_ADMIN_LIMITED must select the new passwordless generator, not the existing LIMITED template")
}

func TestSetupSudoPolicy_RoutesTerminalAdminFullEnumToNewGenerator(t *testing.T) {
	out := contentForAccessLevel(t, pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL)
	want := generateTerminalAdminFullSudoConfig(testTerminalAdminGroup)
	assert.Equal(t, want, out,
		"AccessLevel=TERMINAL_ADMIN_FULL must select the new passwordless generator, not the existing FULL template")
}

func contentForAccessLevel(t *testing.T, level pb.AdminAccessLevel) string {
	t.Helper()
	params := &pb.AdminPolicyParams{
		AccessLevel: level,
		Users:       []string{"cadestro-tty-alice"},
	}
	content, err := sudoConfigForParams(params, testTerminalAdminGroup)
	if err != nil {
		t.Fatalf("sudoConfigForParams: %v", err)
	}
	return content
}

func TestSudoConfig_PassesVisudoCheck_TerminalAdminLimited(t *testing.T) {
	requireVisudo(t)
	out := generateTerminalAdminLimitedSudoConfig(testTerminalAdminGroup)
	requireVisudoAccepts(t, out)
}

func TestSudoConfig_PassesVisudoCheck_TerminalAdminFull(t *testing.T) {
	requireVisudo(t)
	out := generateTerminalAdminFullSudoConfig(testTerminalAdminGroup)
	requireVisudoAccepts(t, out)
}

func requireVisudo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("visudo"); err != nil {
		t.Skipf("visudo not on PATH; skipping syntax-check integration: %v", err)
	}
}

func requireVisudoAccepts(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sudoers.d.cadestro-test")
	if err := os.WriteFile(path, []byte(content), 0o440); err != nil {
		t.Fatalf("write tempfile: %v", err)
	}
	out, err := exec.CommandContext(visudoCtx(t), "visudo", "-c", "-f", path).CombinedOutput()
	if err != nil {
		t.Fatalf("visudo -c -f rejected the generated content:\n%s\n---\n%s", err, out)
	}
}

func visudoCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

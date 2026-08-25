package remote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/network"
)

func TestValidateDestination_RejectsEmptyOrRoot(t *testing.T) {
	for _, p := range []string{"", "/", "  "} {
		t.Run("dest="+p, func(t *testing.T) {
			err := validateDestination(p)
			if !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("validateDestination(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

func TestValidateDestination_RejectsRelative(t *testing.T) {
	for _, p := range []string{"foo", "./foo", "../foo", "subdir/file"} {
		t.Run("dest="+p, func(t *testing.T) {
			err := validateDestination(p)
			if !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("validateDestination(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

func TestValidateDestination_RejectsProtectedPaths(t *testing.T) {
	for _, p := range []string{"/etc", "/boot", "/proc", "/sys", "/usr", "/var", "/bin", "/sbin"} {
		t.Run("dest="+p, func(t *testing.T) {
			err := validateDestination(p)
			if !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("validateDestination(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

func TestValidateDestination_AcceptsNormalAbsolutePaths(t *testing.T) {
	tmp := t.TempDir()
	for _, p := range []string{
		tmp,
		filepath.Join(tmp, "newfile"),
		filepath.Join(tmp, "subdir", "file"),
		"/var/lib/cadestro/something",
		"/etc/cadestro/something",
	} {
		t.Run("dest="+p, func(t *testing.T) {
			if err := validateDestination(p); err != nil {
				t.Fatalf("validateDestination(%q) unexpected err: %v", p, err)
			}
		})
	}
}

func TestValidateDestination_RejectsSymlinkEscape(t *testing.T) {
	tmp := t.TempDir()
	link := filepath.Join(tmp, "escape")
	if err := os.Symlink("/etc", link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	dest := filepath.Join(link, "")
	if err := validateDestination(dest); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("validateDestination(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", dest, err)
	}
}

func TestCanWipe_AllowsManagedRoots(t *testing.T) {
	for _, p := range []string{
		"/var/lib/cadestro/x",
		"/var/lib/cadestro/sub/dir",
		"/etc/cadestro/x",
	} {
		t.Run("dest="+p, func(t *testing.T) {
			if err := canWipe(p); err != nil {
				t.Fatalf("canWipe(%q) unexpected err: %v", p, err)
			}
		})
	}
}

func TestCanWipe_RefusesUnregisteredPath(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "never-recorded")
	if err := canWipe(dest); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("canWipe(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", dest, err)
	}
}

func TestCanWipe_RecordDestRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "recorded-by-fetch")
	t.Cleanup(func() { forgetDest(dest) })

	if err := canWipe(dest); err == nil {
		t.Fatalf("canWipe(%q) before RecordDest returned nil; expected error", dest)
	}
	RecordDest(dest)
	if err := canWipe(dest); err != nil {
		t.Fatalf("canWipe(%q) after RecordDest err: %v", dest, err)
	}
	forgetDest(dest)
	if err := canWipe(dest); err == nil {
		t.Fatalf("canWipe(%q) after forgetDest returned nil; expected error", dest)
	}
}

func TestCanWipe_RefusesProtectedEvenIfRecorded(t *testing.T) {
	cases := append([]string{"/etc", "/var"}, protectedSubpaths...)
	for _, p := range cases {
		t.Run("dest="+p, func(t *testing.T) {
			RecordDest(p)
			t.Cleanup(func() { forgetDest(p) })
			err := canWipe(p)
			if err == nil || !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf("canWipe(%q) = %v; want unsafe rejection", p, err)
			}
		})
	}
}

var protectedSubpaths = []string{
	"/etc/cron.d/evil",
	"/etc/sudoers.d/evil",
	"/etc/systemd/system/evil.service",
	"/usr/bin/sshd",
	"/usr/lib/systemd/system/sshd.service",
	"/bin/ls",
	"/sbin/init",
	"/lib/x86_64-linux-gnu/libc.so.6",
	"/boot/grub/grub.cfg",
	"/home/alice/.ssh/authorized_keys",
	"/home/alice/.bashrc",
	"/root/.bashrc",
	"/var/lib/postgresql/data",
	"/proc/sysrq-trigger",
	"/sys/kernel/uevent_helper",
}

func TestValidateDestination_RejectsProtectedSubpaths(t *testing.T) {
	for _, p := range protectedSubpaths {
		t.Run("dest="+p, func(t *testing.T) {
			if err := validateDestination(p); !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("validateDestination(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

func TestCanWipe_RejectsProtectedSubpaths(t *testing.T) {
	for _, p := range protectedSubpaths {
		t.Run("dest="+p, func(t *testing.T) {
			if err := canWipe(p); !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("canWipe(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

func TestValidateDestination_StillAcceptsManagedRootsUnderProtectedPrefix(t *testing.T) {
	for _, p := range []string{
		"/etc/cadestro",
		"/etc/cadestro/sub/file",
		"/var/lib/cadestro",
		"/var/lib/cadestro/sub/dir/file",
	} {
		t.Run("dest="+p, func(t *testing.T) {
			if err := validateDestination(p); err != nil {
				t.Fatalf("validateDestination(%q) unexpected err: %v", p, err)
			}
		})
	}
}

func TestIsManagedRoot_BoundaryRobustToMissingTrailingSlash(t *testing.T) {
	orig := wipeAllowedRoots
	t.Cleanup(func() { wipeAllowedRoots = orig })
	wipeAllowedRoots = []string{"/etc/cadestro"}

	if isManagedRoot("/etc/cadestro-evil") {
		t.Error("isManagedRoot matched a hostile sibling /etc/cadestro-evil; boundary must not depend on a trailing slash")
	}
	if !isManagedRoot("/etc/cadestro") {
		t.Error("isManagedRoot must match the exact managed root")
	}
	if !isManagedRoot("/etc/cadestro/x") {
		t.Error("isManagedRoot must match a real managed subpath")
	}
}

func TestCanWipe_RejectsManagedRootSiblingPrefix(t *testing.T) {
	for _, p := range []string{"/etc/cadestro-evil/x", "/var/lib/cadestro-evil"} {
		t.Run("dest="+p, func(t *testing.T) {
			if err := canWipe(p); !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("canWipe(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

var agentStateRoots = []string{"/var/lib/cadestro", "/etc/cadestro"}

func TestWipeAllowedRoots_CoverTheDirectoriesTheAgentUses(t *testing.T) {
	if len(wipeAllowedRoots) == 0 {
		t.Fatal("wipeAllowedRoots is empty; the carve-out would refuse every agent-owned path")
	}
	for _, root := range agentStateRoots {
		t.Run("root="+root, func(t *testing.T) {

			for _, p := range []string{root, root + "/state/file"} {
				if !isManagedRoot(p) {
					t.Errorf("isManagedRoot(%q) = false; wipeAllowedRoots %v does not cover the agent's state root",
						p, wipeAllowedRoots)
				}
				if err := canWipe(p); err != nil {
					t.Errorf("canWipe(%q) = %v; the agent cannot clean up its own state", p, err)
				}
				if err := validateDestination(p); err != nil {
					t.Errorf("validateDestination(%q) = %v; the agent cannot write its own state", p, err)
				}
			}

			if isManagedRoot(root + "-evil") {
				t.Errorf("isManagedRoot(%q) matched a hostile sibling of the agent state root", root+"-evil")
			}
		})
	}
}

func TestWipeAllowedRoots_CoverEverythingTheSDKItselfWritesThere(t *testing.T) {
	certDir := network.CertBaseDir + "/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if !isManagedRoot(certDir) {
		t.Fatalf("isManagedRoot(%q) = false; sys/network.CertBaseDir sits outside wipeAllowedRoots %v,"+
			" so the SDK writes EAP-TLS keys into a tree it cannot clean up", certDir, wipeAllowedRoots)
	}
	if err := canWipe(certDir); err != nil {
		t.Fatalf("canWipe(%q) = %v; want nil", certDir, err)
	}
}

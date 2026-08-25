package desktop

import (
	"slices"
	"strings"
	"testing"
)

func TestUserPath(t *testing.T) {
	s := Session{Username: "alice", Home: "/home/alice"}
	got := UserPath(s)
	dirs := strings.Split(got, ":")

	if !slices.Contains(dirs, "/home/alice/.local/bin") {
		t.Errorf("UserPath must include ~/.local/bin, got %q", got)
	}
	if !slices.Contains(dirs, "/usr/bin") {
		t.Errorf("UserPath must include /usr/bin, got %q", got)
	}

	localIdx := slices.Index(dirs, "/home/alice/.local/bin")
	usrIdx := slices.Index(dirs, "/usr/bin")
	if localIdx == -1 || usrIdx == -1 || localIdx > usrIdx {
		t.Errorf("~/.local/bin must precede /usr/bin, got %q", got)
	}

	binIdx := slices.Index(dirs, "/bin")
	for _, sbin := range []string{"/usr/local/sbin", "/usr/sbin", "/sbin"} {
		sbinIdx := slices.Index(dirs, sbin)
		if sbinIdx == -1 {
			t.Errorf("UserPath must include %s (usr-merged default), got %q", sbin, got)
			continue
		}
		if sbinIdx < usrIdx {
			t.Errorf("%s must come after /usr/bin so bin dirs win, got %q", sbin, got)
		}
		if binIdx != -1 && sbinIdx < binIdx {
			t.Errorf("%s must come after /bin, got %q", sbin, got)
		}
	}
}

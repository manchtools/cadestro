package exec

import (
	osexec "os/exec"
	"testing"
)

func TestDetect_ListsOnlyInstalledSudoDoasNeverDirect(t *testing.T) {
	got := Detect(t.Context())

	for _, b := range got {
		if b == Direct {
			t.Errorf("Detect returned Direct; it must list only escalation tools (Sudo/Doas)")
		}
		if b != Sudo && b != Doas {
			t.Errorf("Detect returned unexpected backend %d", b)
		}
	}

	seen := map[PrivilegeBackend]bool{}
	for _, b := range got {
		if seen[b] {
			t.Errorf("Detect returned duplicate backend %d", b)
		}
		seen[b] = true
	}

	_, sudoErr := osexec.LookPath("sudo")
	if (sudoErr == nil) != seen[Sudo] {
		t.Errorf("Detect Sudo presence = %v, but sudo on PATH = %v", seen[Sudo], sudoErr == nil)
	}
	_, doasErr := osexec.LookPath("doas")
	if (doasErr == nil) != seen[Doas] {
		t.Errorf("Detect Doas presence = %v, but doas on PATH = %v", seen[Doas], doasErr == nil)
	}
}

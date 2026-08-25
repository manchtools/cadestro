package fs

import "testing"

func TestIsProtectedPath(t *testing.T) {
	protected := []string{
		"/",
		"/bin",
		"/boot",
		"/dev",
		"/etc",
		"/home",
		"/lib",
		"/lib32",
		"/lib64",
		"/libx32",
		"/media",
		"/mnt",
		"/opt",
		"/proc",
		"/root",
		"/run",
		"/sbin",
		"/srv",
		"/sys",
		"/tmp",
		"/usr",
		"/var",
	}

	for _, p := range protected {
		if !IsProtectedPath(p) {
			t.Errorf("IsProtectedPath(%q) = false, want true", p)
		}
	}

	if !IsProtectedPath("/usr/") {
		t.Error("IsProtectedPath(/usr/) = false, want true")
	}

	if IsProtectedPath("relative/work/dir") {
		t.Error("IsProtectedPath(relative/work/dir) = true, want false")
	}

	notProtected := []string{
		"/opt/myapp",
		"/home/user",
		"/var/log",
		"/tmp/workdir",
		"/usr/local",
		"/data",
		"/custom",
	}

	for _, p := range notProtected {
		if IsProtectedPath(p) {
			t.Errorf("IsProtectedPath(%q) = true, want false", p)
		}
	}
}

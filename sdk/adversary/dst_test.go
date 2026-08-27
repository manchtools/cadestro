package adversary

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/pkg"
	"github.com/manchtools/cadestro/sdk/sys/dns"
	sdkexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
	"github.com/manchtools/cadestro/sdk/sys/network"
)

const defaultDSTSeed = 0x5d_a_da_7a

func dstSeed(t *testing.T) int64 {
	if v := os.Getenv("CADESTRO_DST_SEED"); v != "" {
		s, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("CADESTRO_DST_SEED=%q is not an int64: %v", v, err)
		}
		return s
	}
	return defaultDSTSeed
}

func dstIters() int {
	if v := os.Getenv("CADESTRO_DST_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 4000
}

func adversarialInput(r *rand.Rand) string {
	base := func() string {
		n := r.Intn(12) + 1
		const alpha = "abcdefghijklmnopqrstuvwxyz0123456789.-_+"
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(alpha[r.Intn(len(alpha))])
		}
		return b.String()
	}
	switch r.Intn(16) {
	case 0:
		return ""
	case 1:
		return base()
	case 2:
		return "-cadestroEVIL" + base()

	case 3:
		return "--cadestroEVIL" + base()
	case 4:
		return base() + "\n" + base()
	case 5:
		return base() + "\x00evil"
	case 6:
		return base() + "\t" + base()
	case 7:
		return base() + "; rm -rf /"
	case 8:
		return base() + "$(id)"
	case 9:
		return base() + "`id`"
	case 10:
		return base() + "=" + base()
	case 11:
		return base() + "/" + base()
	case 12:
		return "../../" + base()
	case 13:
		return strings.Repeat("a", 200+r.Intn(400))
	case 14:
		return "/abs/" + base()
	default:
		return base() + string(rune(r.Intn(0x20)))
	}
}

func hasControlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

func checkArgvInvariants(t *testing.T, seed int64, iter int, op, input string, c sdkexec.Command) {
	t.Helper()
	sepAt := -1
	for i, a := range c.Args {
		if a == sdkexec.EndOfOptions {
			sepAt = i
		}

		if hasControlByte(a) {
			t.Fatalf("DST seed=%d iter=%d op=%s input=%q: I1 violated — control char reached argv: %s %q",
				seed, iter, op, input, c.Name, c.Args)
		}

		if a == input && strings.HasPrefix(a, "-") {
			if sepAt == -1 || sepAt >= i {
				t.Fatalf("DST seed=%d iter=%d op=%s input=%q: I2 violated — flag-shaped operand at argv[%d] with no preceding \"--\": %s %q",
					seed, iter, op, input, i, c.Name, c.Args)
			}
		}
	}
}

func TestDST_ArgvAndSecretInvariants(t *testing.T) {
	seed := dstSeed(t)
	iters := dstIters()
	t.Logf("DST argv/secret: seed=%d iters=%d (replay with CADESTRO_DST_SEED=%d)", seed, iters, seed)
	r := rand.New(rand.NewSource(seed))
	ctx := context.Background()
	backends := []pkg.Backend{pkg.Apt, pkg.Dnf, pkg.Pacman, pkg.Zypper}
	totalCmds := 0

	for i := 0; i < iters; i++ {
		input := adversarialInput(r)

		operand := input
		fr := exectest.New(sdkexec.Direct)

		switch r.Intn(6) {
		case 0:
			m, _ := pkg.New(backends[r.Intn(len(backends))], fr)
			_, _ = m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: input})
		case 1:
			m, _ := pkg.New(backends[r.Intn(len(backends))], fr)
			_, _ = m.Remove(ctx, pkg.RemoveOptions{}, input)
		case 2:
			m, _ := pkg.New(backends[r.Intn(len(backends))], fr)
			_, _ = m.InstallLocal(ctx, input, pkg.InstallLocalOptions{})
		case 3:
			m, _ := pkg.New(backends[r.Intn(len(backends))], fr)
			_, _ = m.Search(ctx, input)
		case 4:
			m, _ := pkg.New(backends[r.Intn(len(backends))], fr)
			_, _ = m.Pin(ctx, input)
		case 5:

			operand = ""

			secretVal := "S" + adversarialInput(r) + "Zk9"
			sec, serr := sdkexec.NewSecret(secretVal)
			if serr != nil {
				break
			}
			m, _ := network.New(network.NetworkManager, fr)
			_, _ = m.Apply(ctx, network.Profile{Name: "cadestro-" + sanitize(input), SSID: "net", AuthType: network.AuthPSK, PSK: sec})
			for _, c := range fr.Calls() {
				for _, a := range c.Args {
					if strings.Contains(a, secretVal) {
						t.Fatalf("DST seed=%d iter=%d op=network.Apply: I3 violated — PSK plaintext in argv: %s %q", seed, i, c.Name, c.Args)
					}
				}
			}
		}

		calls := fr.Calls()
		totalCmds += len(calls)
		for _, c := range calls {
			checkArgvInvariants(t, seed, i, "op", operand, c)
		}
	}

	if totalCmds == 0 {
		t.Fatalf("DST seed=%d: zero commands recorded across %d iterations — the driver is vacuous", seed, iters)
	}
	t.Logf("DST argv/secret: %d commands exercised", totalCmds)
}

func TestDST_FaultInjection_HostileHostOutput(t *testing.T) {
	seed := dstSeed(t) ^ 0x1
	iters := dstIters() / 2
	t.Logf("DST fault-injection: seed=%d iters=%d", seed, iters)
	r := rand.New(rand.NewSource(seed))
	ctx := context.Background()
	totalCmds := 0

	for i := 0; i < iters; i++ {
		hostile := adversarialInput(r)
		fr := exectest.New(sdkexec.Direct)
		fr.Push(sdkexec.Result{Stdout: hostile + "\n"}, nil)
		fr.Push(sdkexec.Result{}, nil)
		fr.Push(sdkexec.Result{}, nil)
		fr.Push(sdkexec.Result{}, nil)
		m, _ := dns.New(dns.NetworkManager, fr)
		_ = m.Apply(ctx, dns.Config{Interface: "wlan0", Nameservers: []string{"10.0.0.53"}})

		calls := fr.Calls()
		totalCmds += len(calls)
		for _, c := range calls {

			checkArgvInvariants(t, seed, i, "dns.Apply(hostile-conn-name)", strings.TrimSpace(hostile), c)

			if c.Name == "nmcli" && len(c.Args) > 0 && (c.Args[0] == "connection") {
				for _, a := range c.Args {
					if a == strings.TrimSpace(hostile) && (hasControlByte(a) || strings.HasPrefix(a, "-")) {
						t.Fatalf("DST seed=%d iter=%d: hostile nmcli output reached a privileged connection command: %q", seed, i, c.Args)
					}
				}
			}
		}
	}
	if totalCmds == 0 {
		t.Fatalf("DST fault-injection seed=%d: zero commands recorded — driver is vacuous", seed)
	}
}

func sanitize(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

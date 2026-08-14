package architecture_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDeploymentIsTheThreeServiceTarget(t *testing.T) {
	root := moduleRoot(t)
	compose := readDeploymentFile(t, root, "compose.yml")

	services := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(compose))
	inServices := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "services:" {
			inServices = true
			continue
		}
		if inServices && line != "" && line[0] != ' ' {
			break
		}
		// A comment is not a service. The scanner used to accept any
		// two-space-indented line ending in ':', so the prose introducing the
		// UI service — which ends in a colon — was counted as a service of its
		// own and the exact-set assertion failed with a nonsense name in it.
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if inServices && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			services[strings.TrimSuffix(strings.TrimSpace(line), ":")] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan compose services: %v", err)
	}
	// The deployment ships the administration UI beside control behind the same
	// edge, so the target is three services, not two. The set stays EXACT: an
	// extra service is the failure this guard exists to catch, and widening it
	// to "at least these" is how a reintroduced Valkey or indexer would slip
	// past.
	want := map[string]struct{}{"traefik": {}, "control": {}, "web": {}}
	if len(services) != len(want) {
		t.Fatalf("deployment services = %v; want exactly %v", services, want)
	}
	for service := range want {
		if _, exists := services[service]; !exists {
			t.Errorf("deployment is missing %s", service)
		}
	}

	for _, forbidden := range []string{
		"/var/run/docker.sock",
		"providers.docker",
		"valkey",
		"asynq",
		"indexer",
		"postgres",
	} {
		if strings.Contains(strings.ToLower(compose), forbidden) {
			t.Errorf("compose contains abolished runtime %q", forbidden)
		}
	}
	if !strings.Contains(compose, "internal: true") {
		t.Error("agent proxy network is not isolated")
	}
	if strings.Contains(compose, `http://127.0.0.1:8081/ready`) ||
		!strings.Contains(compose, `https://127.0.0.1:8081/ready`) {
		t.Error("control healthcheck must use its TLS public listener")
	}

	routes := readDeploymentFile(t, root, filepath.Join("traefik", "dynamic", "routes.yml"))
	for _, required := range []string{
		"passthrough: true", "proxyProtocol:", "version: 2", "172.30.0.3:8082",
		"https://control:8081", "serversTransport: control-tls", "serverName: control",
		"rootCAs:", "/run/certs/ca-trust-bundle.crt", "minVersion: VersionTLS13", "maxVersion: VersionTLS13",
	} {
		if !strings.Contains(routes, required) {
			t.Errorf("static route configuration is missing %q", required)
		}
	}
	// Backend transport, per service. The blanket "no `url: http://` anywhere"
	// this replaces stopped expressing the rule the moment the administration
	// UI joined the deployment: that container serves static build output over
	// plain HTTP on the internal compose bridge and holds no secret, so the
	// blanket check could only be satisfied by deleting it.
	//
	// The rule it stands for is still enforced, and now says what it means:
	// every backend hop is TLS unless the service is explicitly excused, and
	// the only excused service is the one whose hop carries nothing worth
	// protecting. Control is NOT excused — its hop carries sessions, secrets,
	// and the agent bridge — so a future edit that downgrades it to http fails
	// here, which the old check would also have caught, while an added
	// plaintext backend for any OTHER service fails too, which it would not
	// have distinguished.
	plaintextExcused := map[string]string{
		"web": "static build output on the internal bridge; no secret crosses this hop",
	}
	backends := traefikHTTPBackends(t, routes)
	if len(backends) == 0 {
		t.Fatal("matches-zero guard: parsed no http.services backends from routes.yml; the parser is broken, not the configuration")
	}
	if _, ok := backends["control"]; !ok {
		t.Error("routes.yml declares no control backend at all")
	}
	for service, urls := range backends {
		if len(urls) == 0 {
			t.Errorf("service %s declares no backend URL", service)
		}
		for _, url := range urls {
			if strings.HasPrefix(url, "https://") {
				continue
			}
			reason, excused := plaintextExcused[service]
			if !excused {
				t.Errorf("service %s uses plaintext backend %q; every backend hop is TLS unless the service is explicitly excused in this test", service, url)
				continue
			}
			// An excuse is about an internal hop. A plaintext URL naming
			// anything but a compose service host has left the bridge the
			// excuse depends on.
			host := strings.TrimPrefix(url, "http://")
			host, _, _ = strings.Cut(host, "/")
			host, _, _ = strings.Cut(host, ":")
			if host != service {
				t.Errorf("service %s is excused plaintext (%s) only for its own internal container; %q points elsewhere", service, reason, url)
			}
		}
	}

	traefik := readDeploymentFile(t, root, filepath.Join("traefik", "traefik.yml"))
	for _, required := range []string{"format: json", "RequestPath: drop", "RequestLine: drop"} {
		if !strings.Contains(traefik, required) {
			t.Errorf("Traefik access-log configuration is missing %q", required)
		}
	}
}

// traefikHTTPBackends maps each service under `http.services` to the backend
// URLs its load balancer declares. It reads the `http:` section only, so the
// `tcp:` agent passthrough — which declares an address, not a URL, and is
// deliberately not terminated here — is out of scope.
func traefikHTTPBackends(t *testing.T, routes string) map[string][]string {
	t.Helper()
	backends := map[string][]string{}
	var section, service string
	inServices := false
	scanner := bufio.NewScanner(strings.NewReader(routes))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch {
		case !strings.HasPrefix(line, " "): // http: / tcp:
			section, inServices, service = strings.TrimSuffix(trimmed, ":"), false, ""
		case !strings.HasPrefix(line, "    "): // routers: / services: / middlewares: …
			inServices, service = strings.TrimSuffix(trimmed, ":") == "services", ""
		case section == "http" && inServices && !strings.HasPrefix(line, "      "):
			service = strings.TrimSuffix(trimmed, ":")
			if _, seen := backends[service]; !seen {
				backends[service] = nil
			}
		case section == "http" && inServices && service != "" && strings.HasPrefix(trimmed, "- url:"):
			backends[service] = append(backends[service], strings.TrimSpace(strings.TrimPrefix(trimmed, "- url:")))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan routes.yml: %v", err)
	}
	return backends
}

func TestReleaseBuildsEachContainerForItsTargetPlatform(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	start := strings.Index(workflow, "  containers:\n")
	end := strings.Index(workflow, "\n  smoke:\n")
	if start < 0 || end <= start {
		t.Fatal("release workflow containers job not found")
	}
	containers := workflow[start:end]
	for _, required := range []string{
		"docker/setup-qemu-action@v3",
		"if: matrix.arch != 'amd64'",
		"platforms: ${{ matrix.arch }}",
		`--platform "linux/${{ matrix.arch }}"`,
	} {
		if !strings.Contains(containers, required) {
			t.Errorf("release containers job is missing %q", required)
		}
	}
}

// moduleRoot is the server module root — server/ — which owns deploy/ and the
// rest of the control plane. This is what the old, misnamed `repositoryRoot`
// always returned: when the server had a repository to itself the two were the
// same directory, and they no longer are.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// repositoryRoot is the monorepo root, above the server module. GitHub honours
// workflows only there, so the release workflow this file asserts against is a
// repository file rather than a server file.
//
// It is discovered by walking up from the module root's parent for a directory
// carrying .github/workflows, not assumed to be one level up: a hardcoded
// depth reports a clean tree the moment the layout moves, and starting above
// the module means a stray server/.github/workflows cannot satisfy the search
// and send this test at a file GitHub never runs.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	module := moduleRoot(t)
	dir := filepath.Dir(module)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".github", "workflows")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate a .github/workflows directory above the module root %s", module)
		}
		dir = parent
	}
}

func readDeploymentFile(t *testing.T, root, path string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, "deploy", path))
	if err != nil {
		t.Fatalf("read deploy/%s: %v", path, err)
	}
	return string(contents)
}

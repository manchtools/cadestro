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
		"rootCAs:", "/run/certs/ca.crt", "minVersion: VersionTLS13", "maxVersion: VersionTLS13",
	} {
		if !strings.Contains(routes, required) {
			t.Errorf("static route configuration is missing %q", required)
		}
	}

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
		case !strings.HasPrefix(line, " "):
			section, inServices, service = strings.TrimSuffix(trimmed, ":"), false, ""
		case !strings.HasPrefix(line, "    "):
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

func TestSmokeProbesEveryComposeServiceAndSharesThePublishedTag(t *testing.T) {
	root := repositoryRoot(t)
	smoke, err := os.ReadFile(filepath.Join(root, "server", "deploy", "smoke-test.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(smoke)
	for _, required := range []string{
		`compose up -d --wait`,
		`for service in "${services[@]}"`,
		`compose exec -T traefik traefik healthcheck --ping`,
		`compose exec -T control wget`,
		`compose exec -T web wget`,
		`CONTROL_SOURCE_IMAGE`,
		`WEB_SOURCE_IMAGE`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("smoke test is missing %q", required)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

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

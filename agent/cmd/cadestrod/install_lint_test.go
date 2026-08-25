package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func readRootFile(t *testing.T, name string) string {
	t.Helper()
	return readRepoFile(t, filepath.Join("..", name))
}

var releaseWorkflow = filepath.Join(".github", "workflows", "release.yml")

func TestInstall_DesktopHandlerOptIn(t *testing.T) {
	sh := readRepoFile(t, "install.sh")

	if !strings.Contains(sh, "--enable-uri-handler") {
		t.Error("install.sh must expose an --enable-uri-handler opt-in flag")
	}
	if !strings.Contains(sh, `if [[ "$ENABLE_URI_HANDLER" == "true" ]]`) {
		t.Error("install_desktop_handler must be gated behind ENABLE_URI_HANDLER (opt-in)")
	}

	if strings.Contains(sh, `ENABLE_URI_HANDLER="${CADESTRO_ENABLE_URI_HANDLER:-true}`) {
		t.Error("the URI handler must default to OFF")
	}

	if strings.Contains(sh, "Terminal=true") {
		t.Error("the desktop entry must not set Terminal=true (drive-by auto-launch)")
	}
}

func TestInstall_TokenDeliveredViaFileNotArgv(t *testing.T) {
	sh := readRepoFile(t, "install.sh")

	if strings.Contains(sh, "-token=$REGISTRATION_TOKEN") {
		t.Error("install.sh must not pass the registration token on argv; use -token-file")
	}
	if !strings.Contains(sh, "-token-file=") {
		t.Error("install.sh enrollment must deliver the token via -token-file")
	}
	if !strings.Contains(sh, `chmod 600 "$token_file"`) {
		t.Error("the install.sh token file must be created mode 0600")
	}
}

func TestInstall_EnrollmentRequiresCAPin(t *testing.T) {
	sh := readRepoFile(t, "install.sh")
	for _, required := range []string{"--pin", "CA_FINGERPRINT_PIN", `-pin=$CA_FINGERPRINT_PIN`} {
		if !strings.Contains(sh, required) {
			t.Errorf("install.sh enrollment is missing %q", required)
		}
	}
}

func TestInstall_VerifiesPublisherSignatureBeforeChecksum(t *testing.T) {
	sh := readRepoFile(t, "install.sh")
	for _, required := range []string{
		"SHA256SUMS.sig", "__RELEASE_SIGNING_PUBLIC_KEY__", "openssl pkeyutl -verify", "verify_release_manifest",
	} {
		if !strings.Contains(sh, required) {
			t.Errorf("install.sh is missing signed-release requirement %q", required)
		}
	}
	signatureCheck := strings.Index(sh, `if ! verify_release_manifest "$tmp_sums" "$tmp_signature" "$tmp_public"; then`)
	hashCheck := strings.Index(sh, `actual_sha=$(sha256sum "$tmp_binary"`)
	if signatureCheck < 0 || hashCheck < 0 || signatureCheck > hashCheck {
		t.Error("publisher signature must be verified before trusting SHA256SUMS")
	}
	if strings.Contains(sh, "CADESTRO_RELEASE_SIGNING_PUBLIC_KEY") {
		t.Error("the published installer's pinned release key must not be replaceable through the environment")
	}
}

func TestReleaseWorkflowSignsChecksumsInProtectedEnvironment(t *testing.T) {
	workflow := readRootFile(t, releaseWorkflow)
	_, releaseJob, ok := strings.Cut(workflow, "\n  release:\n")
	if !ok {
		t.Fatal("release workflow is missing the release job")
	}
	for _, required := range []string{
		"environment: releases", "RELEASE_SIGNING_PRIVATE_KEY", "RELEASE_SIGNING_PUBLIC_KEY",
		"SHA256SUMS.sig", "openssl pkeyutl -sign -rawin", "ED25519 Private-Key:",
	} {
		if !strings.Contains(releaseJob, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
}

func TestReleaseWorkflowPublishesDistinctInstallersAndArchive(t *testing.T) {
	workflow := readRootFile(t, releaseWorkflow)
	for _, required := range []string{"release/cadestro-install.sh", "release/cadestrod-install.sh", "release/cadestro-deploy.tar.gz"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing %q", required)
		}
	}
	if strings.Contains(workflow, "release/"+"install.sh") {
		t.Error("release workflow must not publish a generic install.sh")
	}
}

func TestControlInstallerIsStampedAndDefaultsToItsRelease(t *testing.T) {
	sh := readRootFile(t, "server/deploy/install.sh")
	for _, required := range []string{"__RELEASE_SIGNING_PUBLIC_KEY__", "__INSTALLER_RELEASE_VERSION__", "INSTALLER_RELEASE_VERSION"} {
		if !strings.Contains(sh, required) {
			t.Errorf("control installer is missing %q", required)
		}
	}
	if !strings.Contains(sh, `RELEASE_TAG="$INSTALLER_RELEASE_VERSION"`) {
		t.Error("control installer must default to its stamped release")
	}
}

func TestReleaseWorkflowRunsCanonicalGateOnce(t *testing.T) {
	workflow := readRootFile(t, releaseWorkflow)
	if got := strings.Count(workflow, "bash scripts/verify-all.sh"); got != 1 {
		t.Fatalf("release workflow invokes canonical gate %d times, want 1", got)
	}
}

func TestReleaseWorkflowPrereleaseInstructionsUseExactTag(t *testing.T) {
	workflow := readRootFile(t, releaseWorkflow)
	_, prerelease, ok := strings.Cut(workflow, `if [[ "${{ needs.version.outputs.is_prerelease }}" == "true" ]]; then`)
	if !ok {
		t.Fatal("release workflow is missing the prerelease release-body branch")
	}
	prerelease, _, ok = strings.Cut(prerelease, "          else")
	if !ok {
		t.Fatal("release workflow is missing the stable release-body branch")
	}
	if strings.Contains(prerelease, "releases/latest/download/cadestrod-install.sh") {
		t.Error("prerelease instructions must not bootstrap the latest stable installer")
	}
	if !strings.Contains(prerelease, `releases/download/${TAG}/cadestrod-install.sh`) {
		t.Error("prerelease instructions must use the installer from the exact release tag")
	}

	installers := releaseDownloadRepositories(prerelease)
	if len(installers) < 2 {
		t.Fatalf("matches-zero guard: the prerelease body documents an install and an update installer, so it must carry at least 2 release-download URLs; found %d", len(installers))
	}
	for _, repo := range installers {
		if repo != githubRepositoryExpression {
			t.Errorf("prerelease installer URL hardcodes repository %q; use %s so a fork's release body points at the fork's own signed assets", repo, githubRepositoryExpression)
		}
	}
}

const githubRepositoryExpression = "${{ github.repository }}"

var releaseDownloadRepositoryPattern = regexp.MustCompile(`https://github\.com/(.+?)/releases/(?:latest/)?download/`)

func releaseDownloadRepositories(fragment string) []string {
	var out []string
	for _, m := range releaseDownloadRepositoryPattern.FindAllStringSubmatch(fragment, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestReleaseWorkflowBodyURLsAreForkSafe(t *testing.T) {
	workflow := readRootFile(t, releaseWorkflow)
	_, body, ok := strings.Cut(workflow, "\n      - name: Generate release body\n")
	if !ok {
		t.Fatal("release workflow is missing the release-body step")
	}
	body, _, ok = strings.Cut(body, "\n      - name: ")
	if !ok {
		t.Fatal("release workflow's release-body step is not followed by another step")
	}

	repos := releaseDownloadRepositories(body)
	if len(repos) == 0 {
		t.Fatal("matches-zero guard: extracted no github.com release-download URLs from the release-body step; the pattern is broken")
	}
	for _, repo := range repos {
		if repo != githubRepositoryExpression {
			t.Errorf("release body URL hardcodes repository %q; use %s so forks publish instructions for their own release assets", repo, githubRepositoryExpression)
		}
	}
}

func TestInstall_ReleaseVerifierAcceptsOnlyConfiguredSigner(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is required by the production installer")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte("abc  cadestrod-linux-amd64\n")
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "SHA256SUMS")
	signaturePath := filepath.Join(directory, "SHA256SUMS.sig")
	publicPath := filepath.Join(directory, "release-public.der")
	for path, body := range map[string][]byte{
		manifestPath: manifest, signaturePath: ed25519.Sign(privateKey, manifest), publicPath: publicDER,
	} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	installer := filepath.Join("..", "..", "install.sh")
	if output, err := exec.Command("bash", installer, "--internal-verify-release-manifest", manifestPath, signaturePath, publicPath).CombinedOutput(); err != nil {
		t.Fatalf("valid publisher signature rejected: %v\n%s", err, output)
	}
	if err := os.WriteFile(manifestPath, append(manifest, 'x'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("bash", installer, "--internal-verify-release-manifest", manifestPath, signaturePath, publicPath).Run(); err == nil {
		t.Fatal("installer accepted a manifest modified after signing")
	}
}

func TestInstall_SingleUnitSource(t *testing.T) {
	sh := readRepoFile(t, "install.sh")

	for _, directive := range []string{"CapabilityBoundingSet=", "AmbientCapabilities=", "ExecStart=", "RestrictRealtime=", "[Service]"} {
		if strings.Contains(sh, directive) {
			t.Errorf("install.sh contains unit directive %q — the unit's single source is the embedded template", directive)
		}
	}
	if !strings.Contains(sh, `"$BINARY_PATH" install-unit --data-dir="$DATA_DIR"`) {
		t.Error("install.sh must install the unit via the binary's install-unit subcommand")
	}
	if strings.Contains(sh, "systemctl --version") {
		t.Error("the systemd-version probe moved into the binary; install.sh must not probe")
	}
}

func TestContainerfile_DataDirPerms(t *testing.T) {
	cf := readRepoFile(t, "Containerfile")
	if !strings.Contains(cf, "chmod 700 /var/lib/cadestro") {
		t.Error("Containerfile must `chmod 700 /var/lib/cadestro` after creating it")
	}
}

func TestInstall_PlaceholderAppearsOnlyInTheAssignment(t *testing.T) {
	sh := readRepoFile(t, "install.sh")
	const placeholder = "__RELEASE_SIGNING_PUBLIC_KEY__"

	count := strings.Count(sh, placeholder)
	if count == 0 {
		t.Fatal("install.sh must carry the release-key placeholder assignment; a build with none has nothing to substitute")
	}
	if count != 1 {
		t.Errorf("the release-key placeholder appears %d times; the release sed replaces every occurrence, so only the assignment may carry it", count)
	}
	if !strings.Contains(sh, `RELEASE_SIGNING_PUBLIC_KEY="`+placeholder+`"`) {
		t.Error("the single placeholder occurrence must be the assignment the release sed substitutes")
	}

	substituted := strings.ReplaceAll(sh, placeholder, "TESTKEYBASE64")
	if strings.Contains(substituted, `== "TESTKEYBASE64"`) {
		t.Error("after key substitution the configured-key guard compares the key against itself; assemble the sentinel at run time")
	}
}

func TestInstall_EveryPlaceholderStampedExactlyOnce(t *testing.T) {
	sh := readRepoFile(t, "install.sh")
	wf := readRootFile(t, releaseWorkflow)

	pattern := regexp.MustCompile(`__[A-Z][A-Z0-9_]*__`)
	counts := map[string]int{}
	for _, token := range pattern.FindAllString(sh, -1) {
		counts[token]++
	}
	if len(counts) == 0 {
		t.Fatal("install.sh declares no release placeholders; the release build would stamp nothing")
	}

	substituted := sh
	for token, count := range counts {
		if count != 1 {
			t.Errorf("placeholder %s appears %d times; the global release sed replaces every occurrence, so only its assignment may carry it", token, count)
		}
		if !strings.Contains(sh, `="`+token+`"`) {
			t.Errorf("placeholder %s must appear as a quoted assignment for the release sed to stamp", token)
		}
		if !strings.Contains(wf, token) {
			t.Errorf("release.yml never substitutes %s; a release would ship the raw placeholder and fail closed", token)
		}
		substituted = strings.ReplaceAll(substituted, token, "STAMPED_VALUE")
	}
	if strings.Contains(substituted, `== "STAMPED_VALUE"`) {
		t.Error("after substitution a guard compares a variable against the injected value; assemble sentinels at run time")
	}
}

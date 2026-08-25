package archtest

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var integrationWorkflow = filepath.Join(".github", "workflows", "agent-integration.yml")

func TestCIRunsEveryIntegrationTest(t *testing.T) {
	root := moduleRoot(t)

	files := discoverIntegrationTaggedFiles(t, root)
	if len(files) == 0 {
		t.Fatal("matches-zero guard: discovered no //go:build integration test files; the walk is broken (internal/executor has them today)")
	}

	workflow := filepath.Join(repositoryRoot(t), integrationWorkflow)
	raw, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatalf("read %s: %v", workflow, err)
	}
	pkgs := extractWorkflowPackages(string(raw))
	if len(pkgs) == 0 {
		t.Fatalf("matches-zero guard: extracted no ./agent/<pkg>/ package arguments from %s; the parser is broken", integrationWorkflow)
	}
	selectors := extractRunSelectors(string(raw))
	if len(selectors) == 0 {
		t.Fatalf("matches-zero guard: extracted no -run selectors from %s; the parser is broken", integrationWorkflow)
	}

	for _, p := range pkgs {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p.dir))); err != nil {
			t.Errorf("%s references ./agent/%s/ but that directory does not exist (stale lane entry)", integrationWorkflow, p.dir)
		}
	}

	for file, fns := range files {
		pkg := filepath.ToSlash(filepath.Dir(file))
		if !workflowCovers(pkg, pkgs) {
			t.Errorf("%s carries //go:build integration but package %s is passed to no `go test -tags=integration` invocation in %s — it never even compiles in CI", file, pkg, integrationWorkflow)
			continue
		}
		for _, fn := range fns {
			if fn == "TestMain" {
				continue
			}
			if !matchesAnySelector(fn, selectors) {
				t.Errorf("%s: %s matches none of the workflow's -run selectors %v — it never executes in any CI lane; rename it (TestIntegration_* / TestEdgeCase_*) or add a lane", file, fn, selectors)
			}
		}
	}
}

var inRepoModules = map[string]string{
	"github.com/manchtools/cadestro/contract": "../contract",
	"github.com/manchtools/cadestro/sdk":      "../sdk",
}

func TestIntegrationCIUsesTheInRepoModules(t *testing.T) {
	root := moduleRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inRepoModules) == 0 {
		t.Fatal("matches-zero guard: inRepoModules is empty, so this test asserts nothing")
	}

	for module, want := range inRepoModules {
		if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(module) + `\s+v\S+\s*$`).Match(goMod) {
			t.Errorf("go.mod must require %s", module)
		}
		targets := replaceDirectives(string(goMod), module)
		switch len(targets) {
		case 0:
			t.Errorf("go.mod does not replace %s with %s — without the replace, the placeholder version is resolved from the network instead of from this repository", module, want)
		case 1:
			if targets[0] != want {
				t.Errorf("go.mod replaces %s with %q; it must resolve from %s, the copy in this repository", module, targets[0], want)
			}
		default:
			t.Errorf("go.mod replaces %s %d times (%q); exactly one replace is allowed", module, len(targets), targets)
		}
	}

	files := []string{
		filepath.Join(root, "go.mod"),
		filepath.Join(repositoryRoot(t), integrationWorkflow),
	}
	dockerfiles, err := filepath.Glob(filepath.Join(root, "test", "Dockerfile.integration*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dockerfiles) == 0 {
		t.Fatal("matches-zero guard: discovered no test/Dockerfile.integration* files")
	}
	files = append(files, dockerfiles...)

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, override := range []string{"SDK_MODE", "Resolve SDK branch override", "power-manage-sdk.git", "cadestro-sdk.git"} {
			if strings.Contains(string(raw), override) {
				t.Errorf("the agent must build against the modules in this repository, found out-of-tree resolution %q in %s", override, file)
			}
		}
	}
}

func replaceDirectives(goMod, module string) []string {
	var out []string
	inBlock := false
	for _, raw := range strings.Split(goMod, "\n") {
		line := strings.TrimSpace(stripGoModComment(raw))
		if line == "" {
			continue
		}
		if inBlock {
			if strings.HasPrefix(line, ")") {
				inBlock = false
				continue
			}
		} else {
			rest, ok := afterGoModKeyword(line, "replace")
			if !ok {
				continue
			}
			if strings.HasPrefix(rest, "(") {
				inBlock = true
				line = strings.TrimSpace(strings.TrimPrefix(rest, "("))
				if line == "" || strings.HasPrefix(line, ")") {
					continue
				}
			} else {
				line = rest
			}
		}
		if target, ok := replaceTarget(line, module); ok {
			out = append(out, target)
		}
	}
	return out
}

func stripGoModComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

func afterGoModKeyword(line, keyword string) (string, bool) {
	rest, ok := strings.CutPrefix(line, keyword)
	if !ok || rest == "" {
		return "", false
	}
	if c := rest[0]; c != ' ' && c != '\t' && c != '(' {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func replaceTarget(entry, module string) (string, bool) {
	oldSide, newSide, ok := strings.Cut(entry, "=>")
	if !ok {
		return "", false
	}
	fields := strings.Fields(oldSide)
	if len(fields) == 0 {
		return "", false
	}
	if strings.Trim(fields[0], "\"`") != module {
		return "", false
	}
	return strings.TrimSpace(newSide), true
}

func TestReplaceDirectivesSeesEveryReplaceForm(t *testing.T) {
	const sdkModulePath = "github.com/manchtools/cadestro/sdk"
	const header = "module github.com/manchtools/cadestro/agent\n\ngo 1.25.12\n\nrequire " + sdkModulePath + " v0.0.0\n\n"

	for _, tc := range []struct {
		name   string
		goMod  string
		want   []string
		reason string
	}{
		{
			name:   "go.mod with no replace at all",
			goMod:  header,
			want:   nil,
			reason: "a require without a replace resolves nothing locally, and must not be reported as if it did",
		},
		{
			name:   "single-line replace with a version",
			goMod:  header + "replace " + sdkModulePath + " v0.0.0 => ../sdk\n",
			want:   []string{"../sdk"},
			reason: "the form the original marker scan already caught",
		},
		{
			name:   "single-line replace without a version",
			goMod:  header + "replace " + sdkModulePath + " => ../sdk\n",
			want:   []string{"../sdk"},
			reason: "the version on the left is optional — this is the form the agent uses",
		},
		{
			name:   "block-form replace with a version",
			goMod:  header + "replace (\n\t" + sdkModulePath + " v0.0.0 => ../sdk\n)\n",
			want:   []string{"../sdk"},
			reason: "issue #204: the keyword and the module path sit on different lines",
		},
		{
			name:   "block-form replace with the module path alone on its line",
			goMod:  header + "replace (\n\t" + sdkModulePath + " => ../sdk\n)\n",
			want:   []string{"../sdk"},
			reason: "issue #204: versionless block entry, still a full redirection",
		},
		{
			name:   "block-form replace hidden among unrelated entries",
			goMod:  header + "replace (\n\tgithub.com/other/thing v1.2.3 => ../thing\n\t" + sdkModulePath + " v0.0.0 => github.com/attacker/sdk v0.0.1\n\tgithub.com/third/thing => ../third\n)\n",
			want:   []string{"github.com/attacker/sdk v0.0.1"},
			reason: "the SDK entry must be found regardless of its position in the block, and an out-of-tree target is exactly what the guard rejects",
		},
		{
			name:   "block-form replacing only other modules",
			goMod:  header + "replace (\n\tgithub.com/other/thing v1.2.3 => ../thing\n)\n",
			want:   nil,
			reason: "replacing an unrelated module says nothing about the SDK",
		},
		{
			name:   "module whose path merely starts with the SDK path",
			goMod:  header + "replace " + sdkModulePath + "-extras v1.0.0 => ../extras\n",
			want:   nil,
			reason: "the left-hand path must match exactly, not by prefix",
		},
		{
			name:   "commented-out replace directives",
			goMod:  header + "// replace " + sdkModulePath + " => ../sdk\nreplace (\n\t// " + sdkModulePath + " v0.0.0 => ../sdk\n)\n",
			want:   nil,
			reason: "a commented directive has no effect on the build, so it must not satisfy the required-replace assertion either",
		},
		{
			name:   "block-form replace closed and followed by a require block",
			goMod:  header + "replace (\n\t" + sdkModulePath + " => ../sdk\n)\n\nrequire (\n\tgithub.com/other/thing v1.2.3 // indirect\n)\n",
			want:   []string{"../sdk"},
			reason: "the scanner must leave the block at `)` and not swallow later directives",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceDirectives(tc.goMod, sdkModulePath)
			if len(got) != len(tc.want) {
				t.Fatalf("replaceDirectives() = %q, want %q (%s)", got, tc.want, tc.reason)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("replaceDirectives()[%d] = %q, want %q (%s)", i, got[i], tc.want[i], tc.reason)
				}
			}
		})
	}
}

func discoverIntegrationTaggedFiles(t *testing.T, root string) map[string][]string {
	t.Helper()
	testFn := regexp.MustCompile(`^func (Test[A-Za-z0-9_]*)\(`)
	out := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		tagged := false
		var fns []string
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "//go:build") && buildTagHasIntegration(line) {
				tagged = true
			}
			if m := testFn.FindStringSubmatch(line); m != nil {
				fns = append(fns, m[1])
			}
		}
		if err := sc.Err(); err != nil {
			return err
		}
		if tagged {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = fns
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func buildTagHasIntegration(line string) bool {
	expr := strings.TrimSpace(strings.TrimPrefix(line, "//go:build"))
	for _, tok := range strings.FieldsFunc(expr, func(r rune) bool {
		return r == '&' || r == '|' || r == '(' || r == ')' || r == ' '
	}) {
		if tok == "integration" {
			return true
		}
	}
	return false
}

type workflowPkg struct {
	dir       string
	recursive bool
}

var workflowPkgPattern = regexp.MustCompile(`\./agent/([A-Za-z0-9_/-]+?)/?(\.\.\.)?(\s|\\|$)`)

func extractWorkflowPackages(workflow string) []workflowPkg {
	seen := map[workflowPkg]bool{}
	var out []workflowPkg
	for _, m := range workflowPkgPattern.FindAllStringSubmatch(workflow, -1) {
		p := workflowPkg{dir: strings.TrimSuffix(m[1], "/"), recursive: m[2] == "..."}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

var runSelectorPattern = regexp.MustCompile(`-run[= ]([^\s\\]+)`)

func extractRunSelectors(workflow string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range runSelectorPattern.FindAllStringSubmatch(workflow, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func workflowCovers(pkg string, pkgs []workflowPkg) bool {
	for _, p := range pkgs {
		if pkg == p.dir {
			return true
		}
		if p.recursive && strings.HasPrefix(pkg, p.dir+"/") {
			return true
		}
	}
	return false
}

func matchesAnySelector(fn string, selectors []string) bool {
	for _, s := range selectors {
		re, err := regexp.Compile(s)
		if err != nil {
			continue
		}
		if re.MatchString(fn) {
			return true
		}
	}
	return false
}

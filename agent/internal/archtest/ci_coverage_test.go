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

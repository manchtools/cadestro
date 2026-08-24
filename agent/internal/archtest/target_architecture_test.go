package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentUsesOneFreshManifestSchema(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(moduleRoot(t), "internal", "store", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var migrations []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	// The agent's schema is rooted in ONE fresh, manifest-native baseline.
	// os.ReadDir sorts by name, so requiring that baseline FIRST rejects a
	// second baseline, a renamed one, and anything numbered ahead of it.
	//
	if len(migrations) != 1 || migrations[0] != "001_initial_schema.sql" {
		t.Fatalf("agent schema must contain exactly 001_initial_schema.sql, got %v", migrations)
	}
}

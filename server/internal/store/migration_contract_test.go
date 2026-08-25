package store

import (
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigrationContract(t *testing.T) {
	entries, err := fs.ReadDir(migrationsFS, migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	if len(files) == 0 {
		t.Fatal("server schema has no Goose migrations")
	}
	sort.Strings(files)
	goose.SetBaseFS(migrationsFS)
	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		t.Fatalf("collect Goose migrations: %v", err)
	}
	if len(migrations) != len(files) {
		t.Fatalf("Goose collected %d migrations from %d files", len(migrations), len(files))
	}
	previous := int64(-1)
	for index, name := range files {
		versionText := strings.SplitN(name, "_", 2)[0]
		version, err := strconv.ParseInt(versionText, 10, 64)
		if err != nil || version <= previous {
			t.Fatalf("migration files must have strictly increasing numeric versions: %v", files)
		}
		previous = version
		contents, err := fs.ReadFile(migrationsFS, migrationsDir+"/"+name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		if !strings.Contains(text, "-- +goose Up") {
			t.Errorf("%s has no Goose Up section", name)
		}
		if index > 0 && !strings.Contains(text, "-- +goose Down") {
			t.Errorf("%s has no Goose Down section", name)
		}
	}
}

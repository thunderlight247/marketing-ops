package migrate

import (
	"strings"
	"testing"
	"testing/fstest"
)

func files(entries map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range entries {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestLoadSortsAndParses(t *testing.T) {
	got, err := Load(files(map[string]string{
		"0002_second.sql": "SELECT 2;",
		"0001_first.sql":  "SELECT 1;",
		"README.txt":      "ignored",
	}), ".")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d migrations, want 2", len(got))
	}
	if got[0].Version != 1 || got[0].Name != "first" || got[1].Version != 2 || got[1].Name != "second" {
		t.Errorf("unexpected order or parse: %+v", got)
	}
}

func TestLoadRejectsInvalidSets(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"bad file name", map[string]string{"1_bad.sql": "SELECT 1;"}, "must be named"},
		{"uppercase name", map[string]string{"0001_Bad.sql": "SELECT 1;"}, "must be named"},
		{"duplicate version", map[string]string{"0001_a.sql": "SELECT 1;", "0001_b.sql": "SELECT 2;"}, "duplicate"},
		{"gap", map[string]string{"0001_a.sql": "SELECT 1;", "0003_c.sql": "SELECT 3;"}, "contiguous"},
		{"does not start at one", map[string]string{"0002_b.sql": "SELECT 2;"}, "contiguous"},
		{"empty file", map[string]string{"0001_a.sql": "  \n"}, "empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(files(tc.files), ".")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadMissingDirectory(t *testing.T) {
	if _, err := Load(fstest.MapFS{}, "nope"); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

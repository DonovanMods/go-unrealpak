package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DonovanMods/go-unrealpak"
)

// writeFixturePak builds a small real pak with the library itself, so CLI
// tests exercise the same reader path production callers use.
func writeFixturePak(t *testing.T, mount string, files map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.pak")
	w, err := unrealpak.Create(path, unrealpak.WithMountPoint(mount))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := w.AddFile(name, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun_List_PrintsSortedMountRelativePaths(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"b/second.json": []byte("22"),
		"a/first.json":  []byte("1"),
	})

	var out bytes.Buffer
	if err := run([]string{"list", pak}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	first := strings.Index(got, "a/first.json")
	second := strings.Index(got, "b/second.json")
	if first < 0 || second < 0 {
		t.Fatalf("both entries must be listed; got:\n%s", got)
	}
	if first > second {
		t.Errorf("entries must be sorted by path; got:\n%s", got)
	}
}

func TestRun_List_JSONIsMachineReadable(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json": []byte("1"),
	})

	var out bytes.Buffer
	if err := run([]string{"list", "--json", pak}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	var doc struct {
		MountPoint string `json:"mountPoint"`
		Files      []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if doc.MountPoint != "../../../Game/Content/" {
		t.Errorf("mountPoint = %q", doc.MountPoint)
	}
	if len(doc.Files) != 1 || doc.Files[0].Path != "a/first.json" || doc.Files[0].Size != 1 {
		t.Errorf("files = %+v", doc.Files)
	}
}

func TestRun_Info_ReportsMountAndEntryCount(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json": []byte("1"),
	})

	var out bytes.Buffer
	if err := run([]string{"info", pak}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	for _, want := range []string{"../../../Game/Content/", "entries", "1"} {
		if !strings.Contains(got, want) {
			t.Errorf("info output missing %q; got:\n%s", want, got)
		}
	}
}

func TestRun_UnknownSubcommand_Errors(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"frobnicate"}, &out)
	if err == nil {
		t.Fatal("unknown subcommand must be an error, not a silent no-op")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("error should name the unknown subcommand; got %v", err)
	}
}

func TestRun_NoArgs_Errors(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err == nil {
		t.Fatal("no arguments must be an error")
	}
}

func TestRun_Cat_WritesEntryBytesVerbatim(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json": []byte(`{"hello":"world"}`),
	})

	var out bytes.Buffer
	if err := run([]string{"cat", pak, "a/first.json"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != `{"hello":"world"}` {
		t.Errorf("cat output = %q", out.String())
	}
}

func TestRun_Cat_MissingEntry_Errors(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json": []byte("1"),
	})

	var out bytes.Buffer
	err := run([]string{"cat", pak, "nope.json"}, &out)
	if err == nil {
		t.Fatal("missing entry must be a loud error, never empty output")
	}
	if !strings.Contains(err.Error(), "nope.json") {
		t.Errorf("error should name the missing entry; got %v", err)
	}
}

func TestRun_Extract_WritesFilesAtMountRelativePathsPlusSidecar(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json":  []byte("1"),
		"b/second.json": []byte("22"),
	})
	dir := t.TempDir()

	var out bytes.Buffer
	if err := run([]string{"extract", pak, dir}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	for name, want := range map[string]string{
		"a/first.json":  "1",
		"b/second.json": "22",
	} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("reading extracted %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	mount, err := readSidecar(dir)
	if err != nil {
		t.Fatalf("readSidecar: %v", err)
	}
	if mount != "../../../Game/Content/" {
		t.Errorf("sidecar mountPoint = %q", mount)
	}
}

func TestRun_Extract_FilterSelectsASubset(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json": []byte("1"),
		"b/second.txt": []byte("22"),
	})
	dir := t.TempDir()

	var out bytes.Buffer
	if err := run([]string{"extract", pak, dir, "--filter", "a/*"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "a", "first.json")); err != nil {
		t.Errorf("filtered-in file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b", "second.txt")); !os.IsNotExist(err) {
		t.Errorf("filtered-out file should not exist; stat err = %v", err)
	}
}

// The documented usage puts flags after the positional arguments
// (`unrealpak list <pak> --json`), which Go's flag package does not do on its
// own. Both orders must work.
func TestRun_FlagsWorkBeforeAndAfterPositionals(t *testing.T) {
	pak := writeFixturePak(t, "../../../Game/Content/", map[string][]byte{
		"a/first.json": []byte("1"),
	})

	for _, args := range [][]string{
		{"list", "--json", pak},
		{"list", pak, "--json"},
	} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
		var doc struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		}
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Errorf("run(%v) did not emit JSON: %v\n%s", args, err, out.String())
			continue
		}
		if len(doc.Files) != 1 || doc.Files[0].Path != "a/first.json" {
			t.Errorf("run(%v) files = %+v", args, doc.Files)
		}
	}
}

// A crafted pak could carry an entry path that climbs out of the target
// directory. Extraction must refuse it rather than write outside dir.
func TestRun_Extract_RefusesPathEscape(t *testing.T) {
	dir := t.TempDir()
	if err := checkExtractPath(dir, "../escape.json"); err == nil {
		t.Error("a parent-traversal entry path must be refused")
	}
	if err := checkExtractPath(dir, "/absolute.json"); err == nil {
		t.Error("an absolute entry path must be refused")
	}
	if err := checkExtractPath(dir, "safe/nested.json"); err != nil {
		t.Errorf("an ordinary nested path must be allowed; got %v", err)
	}
}

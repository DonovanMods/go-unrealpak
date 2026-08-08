package main

import (
	"bytes"
	"encoding/json"
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

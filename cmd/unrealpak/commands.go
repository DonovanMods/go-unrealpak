package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/DonovanMods/go-unrealpak"
)

// openPak opens path, wrapping the error with the path so a failure names
// the file the user actually passed.
func openPak(path string) (*unrealpak.Reader, error) {
	r, err := unrealpak.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return r, nil
}

// sortedFiles returns the pak's entries ordered by mount-relative path. The
// index's own order reflects how the pak was written, which is not stable
// across writers; sorting makes CLI output diffable.
func sortedFiles(r *unrealpak.Reader) []unrealpak.FileEntry {
	files := r.Files()
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func cmdInfo(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("info takes exactly one pak path")
	}
	r, err := openPak(fs.Arg(0))
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck

	files := r.Files()
	var total int64
	for _, f := range files {
		total += f.Size
	}
	fmt.Fprintf(out, "mount point: %s\n", r.MountPoint())
	fmt.Fprintf(out, "entries:     %d\n", len(files))
	fmt.Fprintf(out, "total size:  %d bytes\n", total)
	fmt.Fprintf(out, "index hash:  %s\n", r.IndexHash())
	return nil
}

type listJSON struct {
	MountPoint string         `json:"mountPoint"`
	IndexHash  string         `json:"indexHash"`
	Files      []listFileJSON `json:"files"`
}

type listFileJSON struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func cmdList(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("list takes exactly one pak path")
	}
	r, err := openPak(fs.Arg(0))
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck

	files := sortedFiles(r)
	if *asJSON {
		doc := listJSON{MountPoint: r.MountPoint(), IndexHash: r.IndexHash(), Files: make([]listFileJSON, 0, len(files))}
		for _, f := range files {
			doc.Files = append(doc.Files, listFileJSON{Path: f.Path, Size: f.Size})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}
	for _, f := range files {
		fmt.Fprintf(out, "%10d  %s\n", f.Size, f.Path)
	}
	return nil
}

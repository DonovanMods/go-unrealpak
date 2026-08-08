package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DonovanMods/go-unrealpak"
)

// parseArgs parses args against fs and returns the positional arguments,
// tolerating flags that appear after them.
//
// Go's flag package stops parsing at the first non-flag argument, but the
// documented usage puts flags last (`unrealpak extract <pak> <dir> --filter
// '*.json'`), which is what users of tar and friends expect. So parse in a
// loop: each time the parse stops on a positional, set it aside and resume
// on the remainder. Flag values already parsed persist across calls.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		// A literal "--" ends flag parsing for good: everything after it is
		// positional even if it looks like a flag.
		if len(args) > len(rest) && args[len(args)-len(rest)-1] == "--" {
			return append(positional, rest...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

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
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("info takes exactly one pak path")
	}
	r, err := openPak(pos[0])
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
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("list takes exactly one pak path")
	}
	r, err := openPak(pos[0])
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

func cmdCat(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return fmt.Errorf("cat takes a pak path and an entry path")
	}
	r, err := openPak(pos[0])
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck

	data, err := r.ReadFile(pos[1])
	if err != nil {
		return fmt.Errorf("reading %s: %w", pos[1], err)
	}
	_, err = out.Write(data)
	return err
}

// checkExtractPath refuses an entry path that would write outside dir. Pak
// entry paths are attacker-controlled in any archive the user did not build
// themselves, so this is the extract-side equivalent of a zip-slip guard.
func checkExtractPath(dir, entryPath string) error {
	if filepath.IsAbs(entryPath) || strings.HasPrefix(entryPath, "/") {
		return fmt.Errorf("entry %q: absolute paths are not allowed", entryPath)
	}
	target := filepath.Join(dir, filepath.FromSlash(entryPath))
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return fmt.Errorf("entry %q: %w", entryPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("entry %q: escapes the output directory", entryPath)
	}
	return nil
}

func cmdExtract(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	filter := fs.String("filter", "", "only extract entries whose path matches this glob")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return fmt.Errorf("extract takes a pak path and an output directory")
	}
	pakPath, dir := pos[0], pos[1]

	r, err := openPak(pakPath)
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	count := 0
	for _, f := range sortedFiles(r) {
		if *filter != "" {
			ok, err := path.Match(*filter, f.Path)
			if err != nil {
				return fmt.Errorf("bad --filter pattern %q: %w", *filter, err)
			}
			if !ok {
				continue
			}
		}
		if err := checkExtractPath(dir, f.Path); err != nil {
			return err
		}
		data, err := r.ReadFile(f.Path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Path, err)
		}
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		count++
	}

	if err := writeSidecar(dir, r.MountPoint()); err != nil {
		return err
	}
	fmt.Fprintf(out, "extracted %d entries to %s (mount point recorded in %s)\n", count, dir, sidecarName)
	return nil
}

func cmdBuild(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mount := fs.String("mount", "", "mount point to record in the built pak")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return fmt.Errorf("build takes an input directory and an output pak path")
	}
	dir, pakPath := pos[0], pos[1]

	// An explicit --mount always wins; the sidecar is only a convenience for
	// the extract->build cycle. Neither present is a hard error: guessing a
	// mount point produces a pak that loads and silently does nothing.
	mountPoint := *mount
	if mountPoint == "" {
		recorded, err := readSidecar(dir)
		if err != nil {
			return fmt.Errorf("no --mount given and no usable %s in %s: %w "+
				"(pass --mount <mountpoint>)", sidecarName, dir, err)
		}
		mountPoint = recorded
	}

	// os.DirEntry, not fs.DirEntry: the flag.FlagSet above is bound to `fs`,
	// so naming the io/fs package here would be shadowed. os.DirEntry is an
	// alias for the same type, so this needs no extra import at all.
	var entries []string
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == sidecarName {
			return nil // metadata about the pak, never content of it
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", dir, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("%s contains no files to pack", dir)
	}
	sort.Strings(entries)

	w, err := unrealpak.Create(pakPath, unrealpak.WithMountPoint(mountPoint))
	if err != nil {
		return fmt.Errorf("creating %s: %w", pakPath, err)
	}
	for _, rel := range entries {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			w.Close()          //nolint:errcheck
			os.Remove(pakPath) //nolint:errcheck
			return err
		}
		if err := w.AddFile(rel, data); err != nil {
			w.Close()          //nolint:errcheck
			os.Remove(pakPath) //nolint:errcheck
			return fmt.Errorf("adding %s: %w", rel, err)
		}
	}
	if err := w.Close(); err != nil {
		os.Remove(pakPath) //nolint:errcheck
		return fmt.Errorf("finalizing %s: %w", pakPath, err)
	}

	fmt.Fprintf(out, "built %s: %d entries, mount point %s\n", pakPath, len(entries), mountPoint)
	return nil
}

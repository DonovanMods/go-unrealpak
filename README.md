# go-unrealpak

Pure-Go reader and writer for Unreal Engine v11 (`PakFile_Version_Fnv64BugFix`)
`.pak` archives, plus a CLI. No cgo, no external dependencies.

Existing tooling is Rust (repak) or Python (u4pak); this exists because a Go
mod manager needed to read and write these archives in-process.

## Scope

| | |
| --- | --- |
| Read | stored (uncompressed) and Zlib entries |
| Write | stored entries only |
| Oodle | **not supported** — indexes read fine, but reading an Oodle *payload* returns an error naming the entry |
| Encryption | not supported; an encrypted index is rejected |
| Versions | v10+ (the path-hash index format); v11 is what it writes |

The format documentation is in [docs/format.md](docs/format.md), decoded
empirically and verified against 173,078 entries across 34 real paks.

## CLI

```
go install github.com/DonovanMods/go-unrealpak/cmd/unrealpak@latest
```

```
unrealpak info    <pak>                          # mount point, entry count, total size, index hash
unrealpak list    <pak> [--json]                 # entries with sizes, sorted by path
unrealpak cat     <pak> <path>                   # one entry's bytes to stdout
unrealpak extract <pak> <dir> [--filter <glob>]  # entries to dir, + a .unrealpak.json sidecar
unrealpak build   <dir> <pak> [--mount <mount>]  # pack dir; mount defaults to the sidecar's
```

Flags may appear before or after the positional arguments.

`extract` records the source mount point in `.unrealpak.json` so a plain
`extract` → `build` cycle round-trips without flags. `build` refuses to guess a
mount point: a wrong one produces a pak that loads and silently does nothing.

`extract` also refuses any entry path that would escape the output directory,
since entry paths in an archive you did not build yourself are untrusted input.

## Library

```go
r, err := unrealpak.Open("pakchunk0-WindowsNoEditor.pak")
if err != nil {
    return err
}
defer r.Close()

fmt.Println(r.MountPoint(), len(r.Files()))

data, err := r.ReadFile("Engine/Config/Base.ini")
```

```go
w, err := unrealpak.Create("out_P.pak", unrealpak.WithMountPoint("../../../Game/Content/"))
if err != nil {
    return err
}
if err := w.AddFile("data/Thing.json", payload); err != nil {
    return err
}
return w.Close()
```

Mount points and entry paths are whatever you supply — no game's conventions are
baked in.

## Testing

```
go test -race ./...
```

Most reader tests read paks this package's own writer produced. To also exercise
real cooker output (Zlib block lists, 1 MiB alignment padding, a populated
pruned directory index), point `UNREALPAK_TEST_PAK` at a shipped `.pak`:

```
UNREALPAK_TEST_PAK=/path/to/game/Content/Paks/pakchunk0-WindowsNoEditor.pak go test -run RealPak -v
```

That test skips when the variable is unset.

## License

MIT

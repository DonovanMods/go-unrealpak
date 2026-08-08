package unrealpak

import (
	"errors"
	"os"
	"testing"
)

// TestRealPak_DecodesShippedArchive reads a pak produced by a real Unreal
// cooker rather than by this package's writer. Set UNREALPAK_TEST_PAK to a
// shipped .pak to run it, e.g.
//
//	UNREALPAK_TEST_PAK=/path/to/Icarus/Content/Data/data.pak go test -run RealPak -v
func TestRealPak_DecodesShippedArchive(t *testing.T) {
	path := os.Getenv("UNREALPAK_TEST_PAK")
	if path == "" {
		t.Skip("set UNREALPAK_TEST_PAK to a real .pak to run this test")
	}

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	defer r.Close() //nolint:errcheck

	files := r.Files()
	if len(files) == 0 {
		t.Fatal("real pak decoded to zero entries")
	}
	if r.MountPoint() == "" {
		t.Error("real pak decoded to an empty mount point")
	}
	if r.IndexHash() == "" {
		t.Error("index hash is empty")
	}
	t.Logf("%s: mount %q, %d entries", path, r.MountPoint(), len(files))

	// Read every entry the reader claims to support. Unsupported ones
	// (Oodle) must fail with ErrUnsupportedFormat rather than garbage or a
	// panic; anything else read must match its recorded size, which is the
	// per-entry SHA1 gate doing its job.
	var read, unsupported int
	for _, f := range files {
		data, err := r.ReadFile(f.Path)
		if err != nil {
			if errors.Is(err, ErrUnsupportedFormat) {
				unsupported++
				continue
			}
			t.Errorf("ReadFile(%s): %v", f.Path, err)
			continue
		}
		if int64(len(data)) != f.Size {
			t.Errorf("%s: read %d bytes, index says %d", f.Path, len(data), f.Size)
		}
		read++
	}
	t.Logf("read %d entries, %d unsupported (Oodle)", read, unsupported)
	if read == 0 {
		t.Error("no entry was readable; the reader is not exercising real data")
	}
}

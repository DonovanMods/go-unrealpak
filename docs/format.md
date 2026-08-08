# The Unreal Engine v11 `.pak` format

Byte-level specification of `PakFile_Version_Fnv64BugFix` (version 11) `.pak`
archives — the format `go-unrealpak` reads and writes.

**Provenance: this was decoded empirically, not transcribed from Epic
documentation.** Every structure below was recovered by reading the bytes of
real cooker output from a shipped UE 4.25+ title and verified against
**173,078 entries across 34 paks** — not a sample. Where the engine source
(`IPlatformFilePak.h`, `FPakInfo::Serialize`) explains *why* a field sits where
it does, that is noted, but the layouts themselves are what the files actually
contain.

Two claims in this document's own history were falsified by later measurement.
Both corrections are kept in place rather than silently edited out, because the
way each one failed is the useful part — see [Corrections](#corrections).

## Conventions

`FString` is the standard UE string encoding:

```text
int32 Len
Len > 0  ->  Len ANSI bytes, including a trailing NUL
Len < 0  ->  -Len UTF-16LE code units, including a trailing NUL
```

All 34 paks in the corpus use the ANSI form exclusively. All integers are
little-endian.

---

## Part 1 — The footer

The footer sits at the very end of the file. Its size varies with version and
feature gates, so a reader must **locate `Magic` by scanning backward from EOF**
rather than assuming a fixed footer size and reading forward.

Magic is `0x5A6F12E1`, on disk as `E1 12 6F 5A`. A 256-byte tail window is
sufficient to find it for version 11.

### Layout for version 11

Three version gates shape the version-11 footer:

- `PakFile_Version_EncryptionKeyGuid` (7) — adds a 16-byte `EncryptionKeyGuid`
  **before** `Magic`.
- `PakFile_Version_IndexEncryption` (4) — adds the 1-byte `bEncryptedIndex`
  **before** `Magic`, immediately after `EncryptionKeyGuid` when both apply.
- `PakFile_Version_FNameBasedCompressionMethod` (8) — adds a
  `CompressionMethods` array **after** `IndexHash`: 5 slots × 32 bytes, each a
  fixed-width NUL-terminated ASCII name.

```text
EncryptionKeyGuid   (16 bytes)
bEncryptedIndex     (1 byte)
Magic               (4 bytes)   0x5A6F12E1
Version             (4 bytes, int32)
IndexOffset         (8 bytes, int64)
IndexSize           (8 bytes, int64)
IndexHash           (20 bytes)  SHA1 of the primary index region
CompressionMethods  (5 x 32 = 160 bytes)
---
Total: 16+1+4+4+8+8+20+160 = 221 bytes
```

So relative to the magic offset `off`: `bEncryptedIndex` is at `off-1`,
`EncryptionKeyGuid` at `off-17`, and the footer begins at `off-17`.

### Worked example

The last 256 bytes of a 1,421,136,117-byte pak, magic found at absolute offset
1,421,135,913:

```text
00000030: 0000 0000 e112 6f5a 0b00 0000 f6f3 a954  ......oZ.......T
00000040: 0000 0000 d680 0200 0000 0000 5d2f 07c9  ............]/..
00000050: 209f 8a50 f17e 15a3 f656 2e07 d85b 9ab0   ..P.~...V...[..
00000060: 4f6f 646c 6500 0000 0000 0000 0000 0000  Oodle...........
00000080: 5a6c 6962 0000 0000 0000 0000 0000 0000  Zlib............
```

Parsed:

```text
EncryptionKeyGuid: 00000000000000000000000000000000   (all zero: no per-pak key)
bEncryptedIndex:   0
version:           11
index_offset:      1420424182   index_size: 164054
index_hash:        5d2f07c9209f8a50f17e15a3f6562e07d85b9ab0
true footer start: 1421135896   true footer size: 221
compression method slot 0: 'Oodle'
compression method slot 1: 'Zlib'
compression method slots 2-4: '' (zero-padded)
file size:         1421136117
```

The `CompressionMethods` table is **per-pak**. A method index in an entry record
means whatever *that pak's* table says at that slot; see
[Correction 2](#correction-2-the-compression-method-table-is-per-pak).

### SHA1 gate

`IndexHash` is the SHA1 of the `IndexSize` bytes at `IndexOffset`:

```text
sha1(file[1420424182 : 1420424182+164054]) = 5d2f07c9209f8a50f17e15a3f6562e07d85b9ab0
footer IndexHash                            = 5d2f07c9209f8a50f17e15a3f6562e07d85b9ab0   MATCH
```

Verified on all 34 paks.

---

## Part 2 — The index

Version 11 does **not** use a classic flat index (`MountPoint`, `NumEntries`,
then N inline `FPakEntry` records). It uses the UE 4.25+ three-part index: a
primary index holding *bit-packed* entry records plus offsets to two secondary
indexes, each independently SHA1-gated.

### Primary index

At the footer's `IndexOffset`/`IndexSize`, gated by the footer's `IndexHash`.

```text
FString MountPoint
int32   NumEntries
uint64  PathHashSeed
int32   bHasPathHashIndex
  int64  PathHashIndexOffset        // absolute file offset
  int64  PathHashIndexSize
  [20]   PathHashIndexHash          // SHA1 of that region
int32   bHasFullDirectoryIndex
  int64  FullDirectoryIndexOffset
  int64  FullDirectoryIndexSize
  [20]   FullDirectoryIndexHash     // SHA1 of that region
int32   EncodedPakEntriesSize
uint8[] EncodedPakEntries           // bit-packed records, see below
int32   NumNonEncodedFiles          // 0 in all 34 paks; else full FPakEntry records follow
```

Decoded from a real pak:

```text
MountPoint: '../../../'
NumEntries: 9295
PathHashSeed: 0x000000009c4dd25a
bHasPathHashIndex: 1 off=1420588236 size=201687 sha1=52ce88320eae17b776eea8d6e1344d28a19728ac
bHasFullDirectoryIndex: 1 off=1420789923 size=345973 sha1=9a2878db6f62cf566f591c681fa58ba55973ca1e
EncodedPakEntriesSize: 163940 (blob begins 110 bytes into the primary index)
NumNonEncodedFiles: 0
bytes remaining after NumNonEncodedFiles: 0   <- primary index consumed exactly
```

`NumNonEncodedFiles` was 0 in every pak measured. A reader that does not
implement the non-encoded `Files` array should treat a non-zero value as a hard
unsupported-format error rather than ignoring it.

### Region tiling

The four trailing regions tile the end of the file exactly — no gaps, no
padding:

```text
primary    [1420424182, 1420588236)  size=164054   gap_from_prev=-
path-hash  [1420588236, 1420789923)  size=201687   gap_from_prev=0
full-dir   [1420789923, 1421135896)  size=345973   gap_from_prev=0
footer     [1421135896, 1421136117)  size=221      gap_from_prev=0
EOF=1421136117  end_of_last_region=1421136117  gap=0
```

All three SHA1 gates verify on all 34 paks:

```text
sha1(primary index)  = 5d2f07c9209f8a50f17e15a3f6562e07d85b9ab0  == footer IndexHash               MATCH
sha1(path-hash idx)  = 52ce88320eae17b776eea8d6e1344d28a19728ac  == primary PathHashIndexHash      MATCH
sha1(full-dir idx)   = 9a2878db6f62cf566f591c681fa58ba55973ca1e  == primary FullDirectoryIndexHash MATCH
```

### Encoded `FPakEntry` records

Each record starts with a `uint32` flags word:

| bits  | meaning                                                                                     |
| ----- | ------------------------------------------------------------------------------------------- |
| 31    | `Offset` is 32-bit (else 64-bit)                                                            |
| 30    | `UncompressedSize` is 32-bit (else 64-bit)                                                  |
| 29    | `Size` is 32-bit (else 64-bit)                                                              |
| 28-23 | `CompressionMethodIndex` (6 bits; index into the footer's `CompressionMethods`, 0 = stored) |
| 22    | encrypted                                                                                   |
| 21-6  | compression block count (16 bits)                                                           |
| 5-0   | `CompressionBlockSize >> 11`, or `0x3f` = escape (explicit `uint32` follows)                |

Then, in this exact order:

```text
if (flags & 0x3f) == 0x3f : uint32 CompressionBlockSize   // NOTE: precedes Offset
Offset            : uint32 if bit31 else uint64
UncompressedSize  : uint32 if bit30 else uint64
Size              : uint32 if bit29 else uint64           // ONLY when CompressionMethodIndex != 0
                                                          // (when 0, Size == UncompressedSize, not serialized)
if blockCount > 0 && (blockCount > 1 || encrypted):
    blockCount x uint32                                   // per-block compressed length
```

The `CompressionBlockSize`-before-`Offset` ordering is the detail that broke the
first decode attempt. It was recovered by using the directory index's entry
locations as ground truth for record boundaries, then fitting fields to the
observed record widths.

**Verification.** Sequential decode of one pak's blob consumes
**163,940 / 163,940 bytes exactly** and yields **exactly 9295 records ==
`NumEntries`**:

```text
compression-method-index histogram: {0: 4089, 1: 4138, 2: 1068}
encrypted entries: 0
32-bit-safe bits: off32=9295 usz32=9295 sz32=9295   (all entries use the 32-bit forms)
entries with offset+size outside the data region [0,1420424182): 0
multi-block entries whose per-block sizes don't sum to Size: 0
entries carrying an explicit block-size list: 839
record-length histogram: {12: 4089, 16: 18, 20: 4349, 28: 615, 32: 78, 36: 40, 40: 23,
                          44: 28, 48: 12, 52: 5, 56: 5, 60: 3, 64: 9, 76: 2, 88: 2,
                          96: 2, 108: 9, 128: 2, 152: 3, 220: 1}
```

Entries are emphatically **not** all uncompressed. Across all 34 paks the
method-index histogram is `{0: 44744, 1: 127266, 2: 1068}`.

**The stored (`CompressionMethodIndex == 0`) shape is exactly 12 bytes** —
flags word `0xE0000000`, `uint32 Offset`, `uint32 UncompressedSize` — for all
4089 such entries in that pak. That is the record a stored-only writer emits:

```text
000000e0 00000000 ab020000    -> flags=0xE0000000, Offset=0, Size=UncompressedSize=683
```

A compressed record, with block sizes summing to `Size` (verified on every
multi-block entry in all 34 paks):

```text
flags=0xe08000ff -> cmi=1, blocks=3, blkraw=0x3f
  CompressionBlockSize=1048576  Offset=1048576  UncompressedSize=2793472  Size=2584907
  blocks=[996695, 990845, 597367]   sum=2584907 == Size   MATCH
```

### Full directory index

```text
int32 DirCount
repeat DirCount:
    FString DirName          // trailing '/', NO leading '/', except the root dir which is exactly "/"
    int32   FileCount
    repeat FileCount:
        FString FileName     // leaf name only
        int32   PakEntryLocation
```

`PakEntryLocation >= 0` is a byte offset into `EncodedPakEntries`. Negative
values would index the non-encoded `Files` array; **zero negatives were observed
across all 173,078 entries**, so a reader should treat them as a hard
unsupported-format error.

```text
directories=631  path->location mappings=9295 == NumEntries   MATCH
consumed 345973/345973 bytes exactly
every location resolves to a decoded record start: True   negatives: 0
sample dirs: ['Engine/Content/', 'Engine/', '/', 'Engine/Content/EngineResources/']

'Engine/Config/Base.ini'                 -> loc 105252
'Engine/Config/BaseCompat.ini'           -> loc 105264
'Engine/Config/BaseDeviceProfiles.ini'   -> loc 105284
```

The full mount-relative path is `DirName + FileName`, which yields a **leading
`/` only for root-directory files** (`"/" + "DataTableMetadata.json"`). Strip
that leading `/` to get the canonical mount-relative path.

### Path hash index

The path-hash *region* holds two structures back to back:

```text
int32 Count
repeat Count: uint64 PathHash ; int32 PakEntryLocation
<pruned directory index>     // same wire format as the full directory index
```

```text
hash entries=9295 == NumEntries   MATCH
map ends at byte 111544 of 201687
trailing 90143 bytes parse as a second (pruned) directory index:
  dirs=401 files=3696, consuming 201687/201687 exactly
pruned entries are a strict subset of the full index: True
```

**33 of the 34 paks ship an empty pruned directory index** (`DirCount == 0`,
i.e. a bare `int32 0`); only one populates it. Emitting an empty pruned index is
therefore demonstrably a shape the engine loads.

#### The hash recipe

```text
h := 0xCBF29CE484222325 + PathHashSeed        (uint64 wrapping ADD, not XOR)
for each byte b of UTF16LE(lowercase(mount-relative path, leading '/' stripped)):
        h ^= b
        h *= 0x00000100000001B3               (uint64 wrapping multiply)
```

That is standard **FNV-1a 64** with the offset basis *added* to the seed,
hashing the UTF-16LE bytes of the lowercased path with **no NUL terminator**.

This was determined by making the numbers match, not by reading source. A
brute-force sweep over {basis, seed, basis^seed, basis+seed} × {UTF-16LE,
UTF-16LE+NUL, UTF-8, UTF-8+NUL} × {lower, upper, as-is} × {as-is,
strip-leading-`/`, add-leading-`/`, backslashes} left
`basis+seed / UTF-16LE / lower / strip-leading-'/'` as the only recipe that
matches. `basis+seed` and `basis^seed` are distinguishable here — the seeds are
≤ 32 bits and the add carries into bit 32 — and only the ADD form matches.

Verified not on three paths but on **every path in every pak**: each computed
hash is present in the map *and* maps to the same `PakEntryLocation` the
directory index gives.

```text
pak A: 9295/9295     pak B: 298/298     ... all 34 paks: 173,078/173,078   MATCH
```

Worked examples (seed `0x9c4dd25a`):

```text
'Engine/Config/Base.ini'                -> 0xaef1d2bae819faf6 -> loc 105252
'Engine/Config/BaseCompat.ini'          -> 0x53ea9a367ce73eda -> loc 105264
'Engine/Config/BaseDeviceProfiles.ini'  -> 0x6d1b238f9ca74f86 -> loc 105284
```

**The leading-`/` strip is load-bearing.** An initial sweep that concatenated
`DirName + FileName` verbatim passed on 29 of 33 chunks but failed on 5 — in one
case only 111 of 3624 paths matched. Those were precisely the chunks with many
root-directory files, whose paths came out as `/M_DEP_Crate_D.uasset`. Stripping
the leading `/` took every pak to 100%.

No non-ASCII path exists in any of the 34 paks, so ASCII-only versus
full-Unicode case folding is not distinguishable from this corpus. A writer
controls its own paths, so lowercasing ASCII `A-Z` matches UE's
`FChar::ToLower` for anything it emits.

### Per-entry local header

Each file's data region begins with a **full, non-encoded `FPakEntry`**
re-serialized inline, immediately followed by the payload:

```text
int64  Offset                 // ALWAYS 0 in the local copy — not the absolute offset
int64  Size                   // on-disk (post-compression) size
int64  UncompressedSize
int32  CompressionMethodIndex
[20]   Hash                   // SHA1 of the ON-DISK payload bytes
if CompressionMethodIndex != 0:
    int32 BlockCount
    repeat BlockCount: int64 CompressedStart ; int64 CompressedEnd   // relative to entry start
uint8  Flags                  // 0 = not encrypted, not deleted
uint32 CompressionBlockSize
```

**Stored entries: exactly 53 bytes** (8+8+8+4+20+1+4). Compressed entries:
`53 + 4 + 16*BlockCount`.

Note that `Flags` and `CompressionBlockSize` come **after** the block list, not
before it, and that the local `Offset` field is always zero rather than the
entry's absolute offset.

A stored entry's 53-byte header, byte for byte:

```text
00000000 00000000  Offset            = 0
10000000 00000000  Size              = 16
10000000 00000000  UncompressedSize  = 16
00000000           CompressionMethodIndex = 0
a897c5aa2519d4fb9b31c4555aa3a62b297d9e55   Hash (SHA1 of payload)
00                 Flags             = 0
00000000           CompressionBlockSize = 0
```

Cross-check through the full read path
(path → FNV-1a hash → path-hash index → encoded record → local header → payload):

```text
path 'Factions/D_Factions.json'
encoded entry: flags=0xe0000000 offset=816821 size=113 usize=113 cmi=0 rec_len=12
local header (53 B): Offset=0 Size=113 UncompressedSize=113 CMI=0 Flags=0x00 CompressionBlockSize=0
local Hash    = 6b899b02ef58ae54ac64a2f7acf929530c995b29
sha1(payload) = 6b899b02ef58ae54ac64a2f7acf929530c995b29   MATCH
local Size/UncompressedSize agree with the index entry: True
local Offset field is ZERO (not the absolute offset): True
```

`Hash` covers the **on-disk** bytes, proven by fully decompressing a Zlib entry:

```text
path 'Engine/Plugins/Runtime/HairStrands/Config/BaseHairStrands.ini'
cmi=2 blocks=1 size=286 usize=1424 blocksize=1424   local header = 73 bytes (53+4+16)
local blocks (start,end) = [(73, 359)]      <- first block starts exactly at the header end
zlib-decompressed 1424 bytes == UncompressedSize   MATCH
sha1(compressed payload) == local Hash             MATCH
```

Block start/end offsets are **relative to the entry start**, and the first block
begins exactly where the local header ends.

### Data-region packing and alignment

Entries are packed **contiguously** —
`offset(n+1) == offset(n) + headerSize(n) + size(n)` — for 8385 of one pak's
9294 adjacent pairs, and the last entry ends at exactly `IndexOffset`, so the
data region is flush against the index.

The remaining **909 pairs have a padding gap**: every post-gap entry begins at a
**1 MiB-aligned** offset (the cooker's compression-block alignment; 413,219,809
bytes of padding in that pak alone). A reader must therefore always seek to each
entry's recorded `Offset` and never assume contiguity.

A writer may pack contiguously with no alignment at all — 8385 real adjacent
pairs demonstrate that zero padding is accepted.

---

## Corpus-wide verification

The full decode was run against **all 34 paks**, not a spot check. Every one
passes every invariant: version 11, 221-byte footer, `bEncryptedIndex == 0`, all
three SHA1 gates, encoded blob consumed exactly, `NumNonEncodedFiles == 0`, zero
trailing bytes in the primary index, decoded-record count == full-directory-index
count == path-hash count == `NumEntries`, zero out-of-range offsets, all block
sizes summing to `Size`, and 100% path-hash agreement.

```text
pak                     ver entries     dec     fdi     phi sha1x3    hash  blob  oob  blk  pruned
pak A                    11    9295    9295    9295    9295   True    9295  True    0    0    3696 OK
pak B                    11    3078    3078    3078    3078   True    3078  True    0    0       0 OK
pak C                    11   42361   42361   42361   42361   True   42361  True    0    0       0 OK
...  (all 34)  ...
pak Z                    11     298     298     298     298   True     298  True    0    0       0 OK

ALL PAKS PASS: True
```

---

## Corrections

Two claims made during this decode were later falsified by measurement. They are
recorded because each failure mode is a trap a reimplementation can fall into.

### Correction 1: `bEncryptedIndex` sits *before* `Magic`

The first footer parse assumed `bEncryptedIndex` immediately follows
`IndexHash` — the pre-UE4.25 layout. Against a real version-11 pak that yields:

```text
bEncryptedIndex byte: 79
```

`79` is `0x4F`, ASCII `'O'` — the first character of the string `"Oodle"` in the
`CompressionMethods` block. It is not a valid bool byte at all.

`version`, `index_offset`, `index_size`, and `index_hash` all parsed correctly
(the SHA1 gate confirms it) because they are computed forward from `Magic`, and
`Magic` was located correctly. Only the fields *behind* `Magic` were wrong.

A parser reusing that naive layout silently reads garbage for `bEncryptedIndex`
— here it happened to land inside a compression-method name — and misreports the
footer size as 204 instead of 221. Read `bEncryptedIndex` at `magic_offset - 1`
and `EncryptionKeyGuid` at `magic_offset - 17`, going *backward* from the magic,
never forward from the hash.

### Correction 2: the compression-method table is per-pak

A pak of 298 JSON entries was reported as having 258 **Oodle**-compressed
entries, which would have put them out of reach of any pure-Go reader. That
conclusion was wrong, and the error was one of provenance rather than parsing:
the method *indices* were resolved against a **different pak's**
`CompressionMethods` table (`["Oodle","Zlib"]`, where index 1 = Oodle) instead of
the pak's own table, which is `["Zlib"]` — so index 1 means **Zlib** there.

The counts were right; the method name was not. All 258 decompress with the Go
standard library's `compress/zlib`, and all 298 entries reconstruct byte for
byte.

The lesson generalizes: `CompressionMethods` is footer data, and the footer is
per-file. A method index is meaningless without the table from the same pak it
came from. That single mislabel invalidated a chain of downstream decisions
before it was caught.

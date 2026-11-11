# Test fixtures

`solid.cb7` and `plain.cb7` are committed 7z archives used by `archive_test.go`.
Both hold the same two files:

- `a.txt` with the text `alpha`
- `b/c.txt` with the text `charlie`

They were made once with 7-Zip 26.01 from a temporary staging directory that
held those files:

    7z a -t7z -ms=on  testdata/solid.cb7 a.txt b
    7z a -t7z -ms=off testdata/plain.cb7 a.txt b

`-ms=on` puts all files in one solid stream. `-ms=off` gives each file its own
stream. Both archives contain one directory entry (`b`), which the reader must
leave out of the listing.

The CBZ and stored-RAR fixtures are not stored here. The tests build them at run time:
the CBZ with `archive/zip`, and the CBR with the RAR 4.x stored-block writer in
`rar_fixture_test.go`.

## libarchive/

Compressed RAR archives made by WinRAR, used by `rar_real_test.go`. Go cannot
write compressed RAR, so these come from libarchive's test suite
(https://github.com/libarchive/libarchive, `libarchive/test/`), decoded from
their `.uu` form and renamed without the `test_read_format_` prefix. They are
under libarchive's BSD 2-clause license; see `libarchive/COPYING`.

| File | Kind |
|---|---|
| `rar_compress_normal.rar` | RAR4, LZ, normal compression |
| `rar_compress_best.rar` | RAR4, LZ, best compression |
| `rar_filter.rar` | RAR4 with E8 and delta filters |
| `rar_ppmd_lzss_conversion.rar` | RAR4, PPMd and LZ blocks; 177 KB that expands to 242 MB |
| `rar5_compressed.rar` | RAR5 |
| `rar5_solid.rar` | RAR5, solid |
| `rar5_multiple_files_solid.rar` | RAR5, solid, several files |

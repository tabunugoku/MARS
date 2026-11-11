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

The CBZ and CBR fixtures are not stored here. The tests build them at run time:
the CBZ with `archive/zip`, and the CBR with the RAR 4.x stored-block writer in
`rar_fixture_test.go`.

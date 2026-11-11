package archive

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// rarFixtureEntry is one entry for buildRar4. Directories carry no data.
type rarFixtureEntry struct {
	name string // stored with '\' separators, as RAR 4.x does
	data []byte
	dir  bool
}

// rarBlock lays out one RAR 4.x block: HEAD_CRC, HEAD_TYPE, HEAD_FLAGS,
// HEAD_SIZE, then body. HEAD_CRC is the low 16 bits of CRC32 over everything
// from HEAD_TYPE to the end of the header.
func rarBlock(htype byte, flags uint16, body []byte) []byte {
	hdr := make([]byte, 7, 7+len(body))
	hdr[2] = htype
	binary.LittleEndian.PutUint16(hdr[3:], flags)
	binary.LittleEndian.PutUint16(hdr[5:], uint16(7+len(body)))
	hdr = append(hdr, body...)
	binary.LittleEndian.PutUint16(hdr[0:], uint16(crc32.ChecksumIEEE(hdr[2:])))
	return hdr
}

// buildRar4 returns a RAR 4.x archive in which every file is stored (method 0x30).
func buildRar4(entries []rarFixtureEntry) []byte {
	const (
		blockArcHeader = 0x73
		blockFile      = 0x74
		blockEnd       = 0x7b

		flagLongBlock = 0x8000 // header is followed by PACK_SIZE bytes of data
		flagArcNaming = 0x0010 // new-style naming
		flagDirWindow = 0x00e0 // window bits all set marks a directory
		flagEndBlock  = 0x4000
	)
	// DOS date 2020-01-01 00:00:00, in the high 16 bits of FTIME.
	dosTime := uint32((2020-1980)<<25 | 1<<21 | 1<<16)

	var buf bytes.Buffer
	buf.Write([]byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x00}) // marker block
	buf.Write(rarBlock(blockArcHeader, flagArcNaming, make([]byte, 6)))

	for _, e := range entries {
		packSize := uint32(len(e.data))
		unpSize := packSize
		fileCRC := crc32.ChecksumIEEE(e.data)
		flags := uint16(flagLongBlock)
		attr := uint32(0x20)
		if e.dir {
			packSize, unpSize, fileCRC = 0, 0, 0
			flags |= flagDirWindow
			attr = 0x10
		}

		var body []byte
		body = binary.LittleEndian.AppendUint32(body, packSize) // PACK_SIZE
		body = binary.LittleEndian.AppendUint32(body, unpSize)  // UNP_SIZE
		body = append(body, 2)                                  // HOST_OS: Win32
		body = binary.LittleEndian.AppendUint32(body, fileCRC)  // FILE_CRC
		body = binary.LittleEndian.AppendUint32(body, dosTime)  // FTIME
		body = append(body, 20)                                 // UNP_VER
		body = append(body, 0x30)                               // METHOD: stored
		body = binary.LittleEndian.AppendUint16(body, uint16(len(e.name)))
		body = binary.LittleEndian.AppendUint32(body, attr) // ATTR
		body = append(body, e.name...)

		buf.Write(rarBlock(blockFile, flags, body))
		buf.Write(e.data)
	}
	buf.Write(rarBlock(blockEnd, flagEndBlock, nil))
	return buf.Bytes()
}

// writeRar4 writes the fixture to a temp dir as x.cbr and returns its path.
func writeRar4(t *testing.T, entries []rarFixtureEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.cbr")
	if err := os.WriteFile(path, buildRar4(entries), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

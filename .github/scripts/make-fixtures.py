"""Writes a synthetic comic library for the container smoke test.

usage: python3 make-fixtures.py OUT_DIR [SERIES] [BOOKS] [PAGES]

Each series is a folder of CBZ volumes whose pages are generated PNGs
(stdlib only), plus the CBR/CB7 archives from the server's test data, so a
scan exercises every archive reader and thumbnail decoding.
"""

import os
import shutil
import struct
import sys
import zipfile
import zlib

W, H = 800, 1200


def png(seed: int) -> bytes:
    rows = bytearray()
    for y in range(H):
        rows.append(0)  # filter: none
        band = ((y // 40) * 37 + seed * 53) % 256
        for x in range(0, W, 8):
            rows += bytes((band, (x + seed * 11) % 256, (x ^ y) % 256)) * 8

    def chunk(kind: bytes, data: bytes) -> bytes:
        body = kind + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body))

    ihdr = struct.pack(">IIBBBBB", W, H, 8, 2, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(bytes(rows), 6)) + chunk(b"IEND", b"")


def main() -> None:
    out = sys.argv[1]
    series = int(sys.argv[2]) if len(sys.argv) > 2 else 20
    books = int(sys.argv[3]) if len(sys.argv) > 3 else 5
    pages = int(sys.argv[4]) if len(sys.argv) > 4 else 12
    pool = [png(i) for i in range(16)]
    for s in range(1, series + 1):
        folder = os.path.join(out, f"Sample Series {s:02d}")
        os.makedirs(folder, exist_ok=True)
        for b in range(1, books + 1):
            with zipfile.ZipFile(os.path.join(folder, f"Sample Series {s:02d} v{b:02d}.cbz"), "w", zipfile.ZIP_STORED) as z:
                for p in range(1, pages + 1):
                    z.writestr(f"{p:03d}.png", pool[(s * 7 + b * 3 + p) % len(pool)])

    testdata = os.path.join(os.path.dirname(__file__), "..", "..", "server", "internal", "archive", "testdata")
    extra = os.path.join(out, "Archive Formats")
    os.makedirs(extra, exist_ok=True)
    for name in os.listdir(testdata):
        if name.endswith(".cb7"):
            shutil.copy(os.path.join(testdata, name), os.path.join(extra, name))
    rar = os.path.join(testdata, "libarchive")
    for name in os.listdir(rar):
        if name.endswith(".rar"):
            shutil.copy(os.path.join(rar, name), os.path.join(extra, name[:-4] + ".cbr"))
    print(f"wrote {series * books} CBZ books ({series * books * pages} pages) and the CBR/CB7 samples to {out}")


if __name__ == "__main__":
    main()

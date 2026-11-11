# ComiNavi

A lightweight, self-hosted comic and manga server. One static Go binary
(SQLite inside, web UI embedded) that scans your folders, builds series and
books from them, and serves a reader to any browser. No Java, no external
database.

- **Formats:** CBZ / ZIP, CBR / RAR (including RAR5 and solid archives) and
  CB7 / 7z. A folder of archives is a series; `ComicInfo.xml` is imported
  when present.
- **Image:** `ghcr.io/tabunugoku/cominavi`, for `linux/amd64` and
  `linux/arm64`, on distroless (no shell), running as uid 65532.

## Quick start

```sh
docker run -d --name cominavi \
  -p 25600:25600 \
  -v cominavi-data:/data \
  -v /path/to/comics:/comics:ro \
  ghcr.io/tabunugoku/cominavi:latest
```

Open http://localhost:25600 and add a library pointing at `/comics`.

### docker compose

```yaml
services:
  cominavi:
    image: ghcr.io/tabunugoku/cominavi:latest
    container_name: cominavi
    restart: unless-stopped
    ports:
      - "25600:25600"
    environment:
      TZ: Asia/Tokyo
    volumes:
      - ./data:/data              # database, thumbnails, page cache
      - /path/to/comics:/comics:ro
```

The container runs as uid/gid 65532. A named volume is writable out of the
box; a bind-mounted data folder such as `./data` must be owned by that user:

```sh
mkdir -p data && sudo chown 65532:65532 data
```

Comic folders only need to be readable (`:ro` is recommended).

### First-run setup

Until an administrator exists, the sign-in page shows a setup form that asks
for a one-time **setup token**. The server prints it on its first start and
also writes it to `/data/setup-token`:

```sh
docker logs cominavi 2>&1 | grep token=
# or, with a bind-mounted data folder:
sudo cat ./data/setup-token
```

The image has no shell, so `docker exec … cat` does not work; use the log or
the host side of the data folder (`docker cp cominavi:/data/setup-token .`
also works for a named volume).
The token is deleted once the administrator has been created.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `COMINAVI_DATA` | `/data` in the image, `./data` otherwise | Database, thumbnails and page cache |
| `COMINAVI_PORT` | `25600` | Listen port |
| `COMINAVI_HOST` | all interfaces | Listen address, e.g. `127.0.0.1` |
| `COMINAVI_TRUSTED_PROXIES` | none | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is trusted |
| `COMINAVI_DEV` | unset | `1` serves the API only (the Vite dev server serves the UI) |

## Footprint

Measured in CI (`.github/scripts/smoke-image.sh`): first-run setup, then a
scan with thumbnails of a generated library (100 CBZ books of 12 pages, plus
the CBR/CB7 test archives: 21 series, 109 books) on a GitHub-hosted runner.

| | linux/amd64 | linux/arm64 |
| --- | --- | --- |
| Image size (uncompressed) | 8.0 MiB | 7.5 MiB |
| Memory after scan (`docker stats`) | 50.7 MiB | not measured |
| Peak memory during scan (cgroup) | 64 MiB | not measured |

## Building from source

Needs Node 22 and Go (the version in `server/go.mod`). The binary is pure Go
(`CGO_ENABLED=0`), so it cross-compiles anywhere.

```sh
cd web && npm ci && npm run build:embed   # builds the UI into server/internal/webui/dist
cd ../server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cominavi ./cmd/cominavi
./cominavi
```

Or build the image: `docker build -t cominavi .`

## Development

```sh
# terminal 1: API only, data in server/data
cd server && COMINAVI_DEV=1 go run ./cmd/cominavi
# terminal 2: UI with hot reload on http://localhost:5173, proxying /api to :25600
cd web && npm run dev
```

`npm run dev:mock` runs the UI on built-in sample data with no server.
`go test ./...` (in `server/`) and `npm run build` (in `web/`) must pass.
The API contract is `api/openapi.yaml`.

## Releasing

Push a version tag; `.github/workflows/release.yml` builds both architectures
and pushes `X.Y.Z`, `X.Y` (and `X` from 1.0 on) and `latest` to GHCR.
A new GHCR package starts private; make it public in the package settings.

```sh
git tag v0.1.0 && git push origin v0.1.0
```

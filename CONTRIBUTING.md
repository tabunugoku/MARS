# Contributing to MARS

Thank you for helping. Bug reports, feature requests and pull requests are
all welcome.

## Reporting bugs and asking for features

Open an [issue](https://github.com/tabunugoku/MARS/issues/new/choose) and
pick the bug report or feature request form. For a bug, include the image tag
or version you run, how you run it (Docker, TrueNAS, from source), what you
did and what happened. Server logs (`docker logs mars`) help a lot.

Security problems go through a private report instead; see
[SECURITY.md](SECURITY.md).

## Development setup

You need Node 22 and Go (the version in `server/go.mod`).

```sh
# terminal 1: API only, data in server/data
cd server && MARS_DEV=1 go run ./cmd/mars
# terminal 2: web app with hot reload on http://localhost:5173, proxying /api to :25600
cd web && npm ci && npm run dev
```

`npm run dev:mock` (in `web/`) runs the web app on invented sample data
without the Go server.

To build the single binary:

```sh
cd web && npm run build:embed
cd ../server && CGO_ENABLED=0 go build -tags nodynamic -trimpath -ldflags="-s -w" -o mars ./cmd/mars
```

## Layout

- `server/` — Go server (SQLite, scanner, HTTP API, OPDS, Komga-compatible API)
- `web/` — React web app (Vite, Tailwind, shadcn with the Fluid Functionalism registry)
- `api/openapi.yaml` — the API contract. After changing it, run
  `go generate ./internal/httpapi/` in `server/` and `npm run api:types` in `web/`.
- `docs/` — user documentation; `docs/dev/` — design and planning notes
- `site/` — the project website (GitHub Pages)

## Checks

Before opening a pull request, these must pass:

```sh
cd server && go vet ./... && go test ./...
cd web && npm run build && npm run i18n:check && npm run budget
```

## Rules worth knowing

- **Two languages.** Every string in the web app lives in a `defineMessages`
  block with both `ja` and `en`. `npm run i18n:check` fails on Japanese text
  outside a `ja:` block. Server errors carry a `code` and a message in the
  request's language. Details: [docs/dev/i18n.md](docs/dev/i18n.md).
- **Access control.** Every API operation is classified in
  `server/internal/httpapi/access.go` (`opAccess`); an unlisted one is
  refused. Series and books are read only through the builders in
  `query.go`, so library access rules always apply.
- **Design.** The web app follows the rules in
  `.claude/skills/mars-design/SKILL.md`: one accent colour, flat surfaces,
  Keyline icons, and 44 px touch targets on touch screens.
- **Bundle size.** The JavaScript loaded before the first screen stays under
  260 KB gzip, and each screen chunk under 30 KB; `npm run budget` checks it.

## Documentation

User documentation is written in Japanese first (`name.ja.md`) and
translated into English (`name.md`). When you change one, change the other
in the same pull request. If you can only write one language, say so in the
pull request and the other side will be filled in before merging.

## Pull requests

- Keep a pull request to one topic.
- Describe what changes for users and how you tested it.
- By contributing you agree that your contribution is licensed under the
  [MIT License](LICENSE).

# ComiNavi

A lightweight, self-hosted comic/manga server (Komga-class features, no Java).
Backend: Go + SQLite (`server/`, contract in `api/openapi.yaml`). Frontend: React 19 + Vite +
Tailwind v4 + shadcn with the Fluid Functionalism registry (`web/`).

Current phase: **MVP complete** (milestones M0–M5 merged into `main`,
2026-11-12). UI design approved as tag `design/approved-v2`. Distribution:
`Dockerfile` (distroless/static), CI builds and smoke-tests the image,
`release.yml` publishes amd64/arm64 images to GHCR on a `v*` tag (see
README). Next: the 【推奨】 features in `docs/features.md`. Every API operation is classified in
`server/internal/httpapi/access.go` (`opAccess`); an unlisted one is refused,
and series/books are read only through the builders in `query.go`.
The server listens on all interfaces (`COMINAVI_HOST` narrows it); a first
start prints a one-time setup token and writes it to `DATA/setup-token`;
the web app's /login page shows the setup form until an admin exists.
Reader progress writes carry a per-reader `writer` and `seq`; the server
ignores a stale one. `web/` talks to the Go API through
`web/src/api/real.ts`; the invented sample data in `web/src/mock/` loads only
when `VITE_API=mock` (`npm run dev:mock`) and never ships in a normal build.

## Working agreement

- **Opus** (lead session) owns design tokens, shared components, the reader's
  interaction model, screen specs (`docs/screens/`), integration and review.
- **Haiku** subagents implement one screen each from its spec. A Haiku task
  edits only the files its spec lists. It reads
  `.claude/skills/cominavi-design/SKILL.md` first and follows it.
- **Codex** (`codex:codex-rescue`) runs adversarial reviews at milestones.
  Findings are triaged by Opus; fixes go back to Haiku.

## Commands (run in `web/`)

- `npm run dev` — dev server on http://localhost:5173; `/api` is proxied to
  the Go server on :25600 (`COMINAVI_DEV=1 go run ./cmd/cominavi` in `server/`)
- `npm run dev:mock` — the same UI on invented data, no Go server needed
- `npm run build:embed` — production build copied into the Go binary's
  `server/internal/webui/dist`
- `npm run build` — typecheck + production build (must pass before handing back)
- `npm run localize:fluid` — rerun after any `shadcn add --overwrite`: swaps
  the few hard-coded English strings in `components/ui/` for Japanese
- `npm run budget` — production build plus the JS budget check below; exits 1
  when over

## Layout

- `web/src/api/` — types generated from the contract (`npm run api:types`),
  the fetch client (`real.ts`), the mock client (`mock.ts`, dev only)
- `web/src/mock/` — invented sample data (no real works)
- `web/src/components/ui/` — Fluid/shadcn components (don't edit; wrap)
- `web/src/components/shell/` — sidebar, topbar, bottom tabs, theme
- `web/src/components/media/` — Cover, cards, grids, shelves, skeletons
- `web/src/routes/` — one file (or folder) per screen, lazy-loaded
- `web/src/styles/tokens.css` — ComiNavi colour tokens over the Fluid theme
- `docs/features.md` — Komga feature inventory and adoption decisions

## Tools

- Icons: Keyline (`@keyline-icons/react`). For missing glyphs use the
  project MCP server `iconmcp` (`.mcp.json`).
- Fluid's icon slots: `lucide-react` is aliased to a blank stub
  (`src/lib/lucide-stub.ts`), so only the slots mapped in `src/lib/icons.ts`
  draw anything. After installing a Fluid component, grep it for
  `useIcons()` and map each slot it reads to Keyline. An unmapped slot draws
  an empty box and warns `[icons]` in the dev console.
- Skills in this repo: `cominavi-design`, `shadcn`,
  `vercel-react-best-practices`, `web-design-guidelines`.
- Initial JS budget: 260 KB gzip for everything loaded before the first
  screen (the entry chunk and its static imports: React, router, motion,
  Fluid shell), not just the `index-*.js` chunk. Screens are lazy chunks;
  keep each under 30 KB gzip. `npm run budget` checks both.

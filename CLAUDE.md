# ComiNavi

A lightweight, self-hosted comic/manga server (Komga-class features, no Java).
Backend: Go + SQLite (not started yet, `server/`). Frontend: React 19 + Vite +
Tailwind v4 + shadcn with the Fluid Functionalism registry (`web/`).

Current phase: **UI mockup**. `web/` runs on mock data behind `web/src/api/`;
the same code becomes the production frontend once the Go API exists.

## Working agreement

- **Opus** (lead session) owns design tokens, shared components, the reader's
  interaction model, screen specs (`docs/screens/`), integration and review.
- **Haiku** subagents implement one screen each from its spec. A Haiku task
  edits only the files its spec lists. It reads
  `.claude/skills/cominavi-design/SKILL.md` first and follows it.
- **Codex** (`codex:codex-rescue`) runs adversarial reviews at milestones.
  Findings are triaged by Opus; fixes go back to Haiku.

## Commands (run in `web/`)

- `npm run dev` — dev server on http://localhost:5173
- `npm run build` — typecheck + production build (must pass before handing back)
- `npm run localize:fluid` — rerun after any `shadcn add --overwrite`: swaps
  the few hard-coded English strings in `components/ui/` for Japanese

## Layout

- `web/src/api/` — types (the API contract) and the mock-backed client
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
- Skills in this repo: `cominavi-design`, `shadcn`,
  `vercel-react-best-practices`, `web-design-guidelines`.
- Initial JS budget: 260 KB gzip for the shell chunk (React, router, motion,
  Fluid shell). Screens are separate chunks; keep each under 30 KB gzip.

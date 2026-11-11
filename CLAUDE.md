# ComiNavi

A lightweight, self-hosted comic/manga server (Komga-class features, no Java).
Backend: Go + SQLite (`server/`, contract in `api/openapi.yaml`). Frontend: React 19 + Vite +
Tailwind v4 + shadcn with the Fluid Functionalism registry (`web/`).

Current phase: **UI mockup approved** (tag `design/approved-v2`, 2026-11-11;
v2 adds the floating save island). Backend: M0 (foundation) and M1 (scan
and page serving) done; M2 (sign-in and users) is next. The server listens
on 127.0.0.1 until M2 lands. The plan is in milestones M0–M5. `web/` runs on mock data behind `web/src/api/`;
the same code becomes the production frontend once the Go API exists.
Known contract gap for the backend: covers are addressed by mock seeds
(`coverSeed`), not real thumbnail URLs.

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
- `npm run budget` — production build plus the JS budget check below; exits 1
  when over

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

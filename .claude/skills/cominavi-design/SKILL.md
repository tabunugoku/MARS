---
name: cominavi-design
description: ComiNavi's UI rulebook — tokens, shared components, layout, motion, icons and copy rules for every screen in web/. Use before writing or reviewing any UI code in this repo (routes, components, styles), including when implementing a screen spec from docs/screens/.
---

# ComiNavi design system

ComiNavi (Comic Navigator) is a self-hosted comic server. The direction is
**a night chart and a compass, printed in screentone dots**: covers are the
content, the chrome is flat and dark, and the richness comes from dots and
motion that answer what the reader does. Dark ("night chart") is the default;
light ("chart paper") must look equally finished. Everything is flat: no
gradients, glows, blur, glass or coloured shadows.

## Before you write code

1. Read the screen spec in `docs/screens/NN-*.md`. It lists the data, the
   states and the files you may touch. Touch nothing else.
2. Read the shared pieces you will compose (they are short):
   - `web/src/components/media/layout.tsx` — `Page`, `PageHeader`,
     `SectionTitle`, `CoverGrid`, `Shelf`/`ShelfItem`, `EmptyState`
   - `web/src/components/media/cards.tsx` — `SeriesCard`, `BookCard`
   - `web/src/components/media/cover.tsx` — `Cover` (the only cover renderer)
   - `web/src/components/media/skeletons.tsx` — `GridSkeleton`, `Block`
   - `web/src/components/media/brand-action.tsx` — `BrandAction`, the brand CTA
   - `web/src/components/media/kind-icon.tsx` — `KindIcon` (library-kind tile)
   - `web/src/components/brand/app-mark.tsx` — `AppMark` (the logo)
   - `web/src/components/media/dot-track.tsx` — `DotTrack`, progress as dots
   - `web/src/components/media/count-up.tsx` — `CountUp`, numbers that count up
   - `web/src/api/index.ts` — `api.*`, `useApi`, formatters, `statusLabel`
3. UI controls come from `web/src/components/ui/*` (Fluid Functionalism on
   shadcn). Read the component file before using it; props differ from stock
   shadcn. Do not edit files in `components/ui`, `lib/`, `hooks/` — wrap them.

## Hard rules

- **Data only through `@/api`.** Never import `@/mock/*` from a route.
  Every list has three states: loading (skeleton with the final layout's
  shape), empty (`EmptyState` with a next step), and filled.
- **Text uses type roles**: `typeClass("display" | "title" | "subtitle" |
  "body" | "caption" | "micro")` from `@/lib/type-scale`. No raw
  `text-[13px]`, no `text-sm`. Weight via
  `style={{ fontVariationSettings: fontWeights.semibold }}` from
  `@/lib/font-weight` — never `font-bold`/`font-semibold`.
- **Two text colours**: `text-foreground`, `text-muted-foreground`. No
  opacity steps of them. No uppercase, no letter-spacing.
- **Meta in mono**: figures and metadata that read like chart coordinates
  (page "P.46 / 212", sizes, counts, dates in a hero context line) use
  `font-mono` at the caption role, muted. A context line may start with
  `// ` (e.g. `// 前回の続き · 2026.11.09`); at most one per screen.
- **Colours**: `brand` is violet: actions, selection, read progress, the
  primary call to action (at most one per screen). `here` (cyan, `bg-here`,
  `text-here`) marks only "you are here": the current reading position in a
  `DotTrack`, the dot beside the selected nav row, the compass tip. Never use
  `here` for buttons or decoration. Everything else is neutral. No emoji.
- **Dots (screentone)** are the texture: `.tone` (quiet ground) and
  `.tone-strong` (small areas) from `styles/tokens.css`. Progress is a
  `DotTrack`, not a bar, wherever there is room (heroes, book and series
  pages); cover tiles keep their thin bar.
- **Icons**: `@keyline-icons/react` only (stroke style, size 16 in rows,
  14 in captions, 18 in tabs). If Keyline lacks a glyph, look it up with the
  IconMCP server; never mix in a second icon style. Check the name exists:
  `node -e "console.log(!!require('@keyline-icons/react').Name)"` in `web/`.
  `KindIcon` is the kind's Keyline glyph on a violet screentone tile, for
  library headers and the admin library list.
- **Buttons with an icon** pass it as `leadingIcon={X as IconComponent}`, never
  as a child: children become one label span, so an icon child stacks above
  the text.
- **Spacing** on the 4px grid via Tailwind (`gap-2`, `p-4`, …). Page gutter
  and max width come from `Page`; don't add your own outer padding.
- **Covers** are always `Cover`/`SeriesCard`/`BookCard`; grids are always
  `CoverGrid`; horizontal rows are `Shelf` + `ShelfItem`.
- **Motion**: springs for anything that moves (`spring.*` from
  `@/lib/springs`, or motion/react spring options); CSS transitions use
  80–300ms with `ease-[cubic-bezier(0.16,1,0.3,1)]` (glide) or
  `ease-[cubic-bezier(0.34,1.45,0.64,1)]` (a lift that overshoots a little).
  Animate transform/opacity only. Respect reduced motion. Allowed on load:
  the dots of a `DotTrack` filling in and `CountUp` numbers; nothing flies in.
  No looping motion except loading states. Already built in, so don't
  re-implement: card hover lift with the violet dot spotlight
  (`SeriesCard`/`BookCard`), the compass backdrop that turns per route, the
  gliding cyan dot on the active nav row, dot-wave skeletons, the screentone
  wipe on theme switch (call `toggle(buttonEl)` from `useTheme`).
- **Toasts**: `toast.success(...)` (an object, not a function) from `@yusufceng1z/ding-react` for confirmations
  ("既読にしました", "スキャンを開始しました"). One short sentence.
- **Accessibility**: real `<button>`/`<a>`; every icon-only button has an
  `aria-label` in Japanese and a `Tooltip`; visible focus
  (`focus-visible:ring-2 ring-[var(--focus-ring)]`); hit targets ≥ 36px
  desktop, ≥ 44px on touch.
- **Responsive**: design at 1280px, verify at 375px. Below `md` the bottom
  tab bar is present (shell adds `pb-20`); dialogs become full-width.
- **Copy** is Japanese, plain and short (natural-japanese skill). Buttons
  are verbs ("読む", "続きから読む", "スキャン"). No exclamation marks.
- Stand-in art comes from `@/lib/art` (`coverArt`, `pageArt`). Never link
  external images.

## Done means

- `npm run build` in `web/` passes with no type errors.
- The screen renders all three states (force empty by filtering mock data
  in a scratch call, then revert).
- No new dependency unless the spec says so.
- Report back: files changed, anything in the spec you could not do, and
  any shared component you think needs a change (don't change it yourself).

## References

`docs/design-references.md` explains which reference site informed which
rule. Deeper craft for Fluid components:
`~/.claude/skills/fluid-functionalism/references/craft.md`.

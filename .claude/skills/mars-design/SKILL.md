---
name: mars-design
description: MARS's UI rulebook — tokens, shared components, layout, motion, icons and copy rules for every screen in web/. Use before writing or reviewing any UI code in this repo (routes, components, styles), including when implementing a screen spec from docs/dev/screens/.
---

# MARS design system

MARS ("MARS: A Reading Server") is a self-hosted comic
server. Tagline: "Your manga. Your server. Your universe." (always English). The direction is
**a night chart of Mars, printed in screentone dots**: covers are the
content, the chrome is flat and dark, and the richness comes from dots and
motion that answer what the reader does. Dark ("night chart") is the default;
light ("chart paper") must look equally finished. Surfaces, text and chrome
are flat: no gradients, blur, glass or coloured shadows. The one exception is
**light from the chart itself**, all of it in `web/src/styles/glow.css`: the
blue "you are here" marks glow like a beacon, the red route already read
(`DotTrack`, progress bars, the reader rail) gives off a little light, the
brand CTA has a small halo, and a lifted cover throws red light below
it. The logo is flat, never lit. Those come for free from `bg-here`, `bg-brand`, `BrandAction` and the
cards, so never add a glow by hand, and never light anything else (cards,
panels, headings, icons, secondary buttons).

## Before you write code

1. Read the screen spec in `docs/dev/screens/NN-*.md`. It lists the data, the
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
  `font-mono` at the caption role, muted. Only the figure is mono; the words
  around it stay in the UI face (`前回の続き · ` + mono `2026.11.09`). No
  `// ` code-comment prefixes or other eyebrows: this is a reading app, not a
  developer tool.
- **Colours**: `brand` is Martian oxide red: actions, selection, read
  progress, the primary call to action (at most one per screen). Text and
  marks use `text-brand`/`bg-brand`; anything with white text on it (the CTA,
  a filled checkbox, a count badge) uses `bg-brand-fill`, a darker red that
  keeps white text readable in both themes. Errors and destructive actions
  use `destructive` (crimson), never the brand red. `here` (blue, `bg-here`,
  `text-here`) marks only "you are here": the current reading position in a
  `DotTrack`, the dot beside the selected nav row, the near moon of the
  sign-in page's planet (`PlanetOrbit`, the only large drawing; app screens keep only the
  `.tone` ground behind the content). Never use
  `here` for buttons or decoration. Everything else is neutral. No emoji.
- **Dots (screentone)** are the texture: `.tone` (quiet ground) and
  `.tone-strong` (small areas) from `styles/tokens.css`. Progress is a
  `DotTrack`, not a bar, wherever there is room (heroes, book and series
  pages); cover tiles keep their thin bar.
- **Icons**: `@keyline-icons/react` only (stroke style, size 16 in rows,
  14 in captions, 18 in tabs). If Keyline lacks a glyph, look it up with the
  IconMCP server; never mix in a second icon style. Check the name exists:
  `node -e "console.log(!!require('@keyline-icons/react').Name)"` in `web/`.
  `KindIcon` is the kind's Keyline glyph on a red-tint tile, for
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
  80–300ms with the one glide curve `ease-[cubic-bezier(0.16,1,0.3,1)]`.
  No overshooting curves on hover or press; overshoot belongs only to the
  springs that model something physical (page turns, the moon settling on the sign-in orbit).
  Animate transform/opacity only. Respect reduced motion. Allowed on load:
  the dots of a `DotTrack` filling in and `CountUp` numbers; nothing flies in.
  No looping motion except loading states. Already built in, so don't
  re-implement: card hover lift with the red dot spotlight
  (`SeriesCard`/`BookCard`), the gliding blue dot on the active nav row, dot-wave skeletons, the screentone
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

`docs/dev/design-references.md` explains which reference site informed which
rule. Deeper craft for Fluid components:
`~/.claude/skills/fluid-functionalism/references/craft.md`.

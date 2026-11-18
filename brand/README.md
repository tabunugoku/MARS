# MARS wordmark

The text logo that goes with the planet icon (`web/public/favicon.svg`).

| File | Use |
| --- | --- |
| `mars-wordmark.svg` | The main logo: two colours, on dark backgrounds |
| `mars-wordmark-mono.svg` | One colour (`currentColor`) with a gutter instead of the shift: print, watermarks, light backgrounds |
| `mars-wordmark-small.svg` | No cut, `currentColor`: below 80 px wide, where the cut no longer reads |

## Design

- Typeface: [Unbounded](https://github.com/google/fonts/tree/main/ofl/unbounded) at weight 600, SIL Open Font License 1.1. The letters are outlined to paths, so the SVGs need no font.
- Spacing: 70 units (of 1000) after each letter, R–S 40 units tighter.
- The cut: one straight line through the middle of the cap height, rising 2.5° to the right, like a gutter between manga panels. Steeper cuts drop too far across the word and leave slivers on M and S.
- Main logo: above the cut in the icon's star colour `#f3c9bd`, below it in the planet colour `#e8542f`, shifted 70 units along the cut.
- Mono logo: the same cut as a 70-unit gap, no shift.
- Minimum size: the cut reads down to about 80 px wide; use the small version below that.
- Clear space: at least the height of the icon's moon dot (about a third of the cap height) on every side.
- Lockup: the icon to the left of the wordmark, the icon about 1.4 times the cap height, the gap about half the icon's width.

## Rebuilding

`build_wordmark.py` holds every number above. It needs fontTools and a copy of `Unbounded[wght].ttf`, which is not in the repository:

```bash
python brand/build_wordmark.py path/to/Unbounded[wght].ttf
```

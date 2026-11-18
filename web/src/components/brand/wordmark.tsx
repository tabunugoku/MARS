// The MARS wordmark: Unbounded 600 cut once like a gutter between manga
// panels. Above the cut in --wordmark-ink (the icon's star dust on the dark
// theme, the text colour on the light one), below it in --brand, shifted along
// the cut. Below about 80 px wide the cut no longer reads, so the letters are
// drawn whole. Paths come from brand/build_wordmark.py; see brand/README.md.

import { useId } from "react";
import {
  WORDMARK_BOTTOM,
  WORDMARK_LETTERS,
  WORDMARK_ORIGIN,
  WORDMARK_SHIFT,
  WORDMARK_TOP,
  WORDMARK_VIEWBOX,
} from "./wordmark-paths";

const [, , VB_W, VB_H] = WORDMARK_VIEWBOX.split(" ").map(Number);
const MIN_CUT_WIDTH = 80;

function Letters() {
  return (
    <>
      {WORDMARK_LETTERS.map((d, i) => (
        <path key={i} d={d} />
      ))}
    </>
  );
}

/** height is in px; the width follows the logo's proportions. Without a
 * label the logo is decorative (aria-hidden), for places that already name
 * the app, such as a link with its own aria-label. */
export function Wordmark({ height = 20, label, className }: { height?: number; label?: string; className?: string }) {
  const id = useId();
  const width = (height * VB_W) / VB_H;
  const cut = width >= MIN_CUT_WIDTH;
  const a11y = label ? { role: "img", "aria-label": label } : { "aria-hidden": true };
  return (
    <svg
      data-wordmark
      width={width}
      height={height}
      viewBox={WORDMARK_VIEWBOX}
      className={className}
      focusable="false"
      {...a11y}
    >
      {cut ? (
        <>
          <defs>
            <clipPath id={`${id}t`}>
              <polygon points={WORDMARK_TOP} />
            </clipPath>
            <clipPath id={`${id}b`}>
              <polygon points={WORDMARK_BOTTOM} />
            </clipPath>
          </defs>
          <g transform={WORDMARK_ORIGIN}>
            <g clipPath={`url(#${id}t)`} fill="var(--wordmark-ink)">
              <Letters />
            </g>
            <g transform={WORDMARK_SHIFT}>
              <g clipPath={`url(#${id}b)`} fill="var(--brand)">
                <Letters />
              </g>
            </g>
          </g>
        </>
      ) : (
        <g transform={WORDMARK_ORIGIN} fill="var(--wordmark-ink)">
          <Letters />
        </g>
      )}
    </svg>
  );
}

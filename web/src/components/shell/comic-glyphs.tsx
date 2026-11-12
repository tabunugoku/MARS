// The comics glyph (a hero mask) is iconmcp's "hero-mask": Keyline has no comic glyph.
import type { ReactNode, SVGProps } from "react";

import type { IconComponentProps } from "@/lib/icon-context";

type GlyphProps = IconComponentProps & SVGProps<SVGSVGElement>;

function Glyph({ size = 16, strokeWidth = 1.5, className, children, ...props }: GlyphProps & { children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 16 16"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      className={className}
      {...props}
    >
      {children}
    </svg>
  );
}

export function ComicsGlyph(props: GlyphProps) {
  return (
    <Glyph {...props}>
      <path d="M1.75 5.75 C4.25 5.25 6.25 5.75 8 7 C9.75 5.75 11.75 5.25 14.25 5.75 C14 9 12.75 11 10.5 11 C9.25 11 8.5 10.25 8 9.5 C7.5 10.25 6.75 11 5.5 11 C3.25 11 2 9 1.75 5.75 Z" />
      <path d="M3.75 7.75 C4.75 7.5 6 7.75 6.5 8.5 C5.75 9.25 4.5 9.25 3.75 7.75 Z" />
      <path d="M12.25 7.75 C11.25 7.5 10 7.75 9.5 8.5 C10.25 9.25 11.5 9.25 12.25 7.75 Z" />
    </Glyph>
  );
}

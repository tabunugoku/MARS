import type { ComponentType, ReactNode } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";

/** The one brand-coloured call to action per screen ("続きから読む", "読む").
 *  A link, because it always navigates to the reader or a page. Matches the
 *  Fluid button's 36px height and press feel (1px inset on :active). */
export function BrandAction({
  to,
  children,
  icon: Icon,
  className,
  block,
}: {
  to: string;
  children: ReactNode;
  icon?: ComponentType<{ size?: number }>;
  className?: string;
  block?: boolean;
}) {
  return (
    <Link
      to={to}
      className={cn(
        "inline-flex h-9 items-center justify-center gap-1.5 rounded-lg bg-brand px-4 text-brand-foreground",
        "transition-[filter,transform] duration-200 ease-[cubic-bezier(0.34,1.45,0.64,1)] hover:brightness-110 active:scale-[0.96] active:brightness-95 motion-reduce:transform-none",
        "outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        typeClass("body"),
        block && "w-full",
        className,
      )}
      style={{ fontVariationSettings: fontWeights.semibold }}
    >
      {Icon && <Icon size={16} />}
      {children}
    </Link>
  );
}

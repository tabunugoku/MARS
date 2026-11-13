import { useId, useState, type ReactNode } from "react";
import { ChevronDown } from "@keyline-icons/react";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

// Filter controls of the library screen that take lists of options. The
// labels come in as props: this file holds no copy of its own.

/** A list with more options than this gets a search box above it. */
const SEARCH_FROM = 8;

/** One filter category, folded by its heading. It starts open on wide screens
 *  and on a phone only when something in it is chosen, so the panel does not
 *  fill a small screen; `count` (how many are chosen) shows beside the title. */
export function FilterGroup({ title, count = 0, children }: { title: string; count?: number; children: ReactNode }) {
  const [open, setOpen] = useState(() => count > 0 || window.matchMedia("(min-width: 48rem)").matches);
  const body = useId();
  return (
    <div className="space-y-2">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={body}
        onClick={() => setOpen((o) => !o)}
        className={cn(
          typeClass("caption"),
          "touch-hit relative flex w-full items-center gap-2 text-left text-muted-foreground hover:text-foreground",
        )}
      >
        <span className="flex-1">
          {title}
          {count > 0 && <span className="ml-1.5 font-mono tabular-nums text-foreground">{count}</span>}
        </span>
        <ChevronDown size={14} className={cn("shrink-0 transition-transform duration-80", open && "rotate-180")} />
      </button>
      <div id={body} hidden={!open} className="flex flex-col gap-2">
        {children}
      </div>
    </div>
  );
}

export function CheckRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: () => void }) {
  return (
    <label className={cn(typeClass("body"), "touch-hit relative flex min-h-9 cursor-pointer items-center gap-2 text-foreground")}>
      <Checkbox checked={checked} onCheckedChange={onChange} />
      {label}
    </label>
  );
}

/** Multi-select over a list of options. Long lists filter as you type and
 *  scroll inside their own box, so the panel stays short on a phone. */
export function CheckList({
  options,
  selected,
  label,
  onToggle,
  searchLabel,
  noMatch,
}: {
  options: string[];
  selected: string[];
  label?: (value: string) => string;
  onToggle: (value: string) => void;
  searchLabel: string;
  noMatch: string;
}) {
  const [query, setQuery] = useState("");
  const text = (v: string) => (label ? label(v) : v);
  const q = query.trim().toLowerCase();
  const shown = q ? options.filter((o) => text(o).toLowerCase().includes(q)) : options;
  return (
    <>
      {options.length > SEARCH_FROM && (
        <Input
          type="search"
          aria-label={searchLabel}
          placeholder={searchLabel}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      )}
      <div className="flex max-h-64 flex-col gap-2 overflow-y-auto p-0.5">
        {shown.map((o) => (
          <CheckRow key={o} label={text(o)} checked={selected.includes(o)} onChange={() => onToggle(o)} />
        ))}
        {shown.length === 0 && <p className={cn(typeClass("caption"), "text-muted-foreground")}>{noMatch}</p>}
      </div>
    </>
  );
}

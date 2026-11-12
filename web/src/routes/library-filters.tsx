import { useState, type ReactNode } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

// Filter controls of the library screen that take lists of options. The
// labels come in as props: this file holds no copy of its own.

/** A list with more options than this gets a search box above it. */
const SEARCH_FROM = 8;

export function FilterGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <p className={cn(typeClass("caption"), "text-muted-foreground")}>{title}</p>
      <div className="flex flex-col gap-2">{children}</div>
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

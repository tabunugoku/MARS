// Keyline Icons (MIT, 24px grid, 2px stroke) as the app-wide icon set.
// Only the Fluid slots our installed components actually read are mapped:
// every icon listed here ships in the startup chunk. Lucide is aliased to a
// blank stub (lib/lucide-stub.ts), so after installing a Fluid component,
// grep it for useIcons() and add its slots here, or it draws nothing (dev
// builds warn in the console). App icons import Keyline directly.
import type { IconComponent, IconName } from "@/lib/icon-context";
import {
  ChevronDown,
  ChevronRight,
  ChevronsUpDown,
  CircleAlert,
  File,
  Minus,
  PanelLeft,
  PanelRight,
  Plus,
  Search,
  X,
} from "@keyline-icons/react";

export const keylineSlots: Partial<Record<IconName, IconComponent>> = {
  "search": Search as IconComponent,
  "chevrons-up-down": ChevronsUpDown as IconComponent,
  "chevron-down": ChevronDown as IconComponent,
  "chevron-right": ChevronRight as IconComponent,
  "x": X as IconComponent,
  "file": File as IconComponent,
  "minus": Minus as IconComponent,
  "plus": Plus as IconComponent,
  "panel-left": PanelLeft as IconComponent,
  "panel-right": PanelRight as IconComponent,
  "circle-alert": CircleAlert as IconComponent,
};

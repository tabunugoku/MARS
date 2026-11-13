// Stand-in for lucide-react, wired in vite.config.ts (resolve.alias).
// Fluid's lib/icon-context imports Lucide for its default icon map. MARS
// maps every slot it uses to Keyline (lib/icons.ts), so the Lucide drawings
// would ship at startup and never show. Each name here is a blank 24px icon
// instead; in development it warns once, so a slot that was never mapped to
// Keyline gets noticed. If Fluid starts importing a name missing below, the
// build fails on the missing export: add it here and map the slot.
import { createElement, type SVGProps } from "react";

type Props = SVGProps<SVGSVGElement> & { size?: number | string };
const warned = new Set<string>();

function stub(name: string) {
  function Icon({ size = 24, ...rest }: Props) {
    if (import.meta.env.DEV && !warned.has(name)) {
      warned.add(name);
      console.warn(`[icons] A Fluid slot rendered Lucide "${name}". Map the slot to Keyline in src/lib/icons.ts.`);
    }
    return createElement("svg", { width: size, height: size, viewBox: "0 0 24 24", "aria-hidden": true, ...rest });
  }
  Icon.displayName = `LucideStub(${name})`;
  return Icon;
}

export const ChevronRight = stub("ChevronRight");
export const ChevronDown = stub("ChevronDown");
export const X = stub("X");
export const Copy = stub("Copy");
export const Menu = stub("Menu");
export const Dot = stub("Dot");
export const Monitor = stub("Monitor");
export const Sun = stub("Sun");
export const Moon = stub("Moon");
export const RectangleHorizontal = stub("RectangleHorizontal");
export const Circle = stub("Circle");
export const Square = stub("Square");
export const SquareLibrary = stub("SquareLibrary");
export const Clock = stub("Clock");
export const Star = stub("Star");
export const Settings = stub("Settings");
export const Plus = stub("Plus");
export const ArrowLeft = stub("ArrowLeft");
export const ArrowRight = stub("ArrowRight");
export const ArrowUp = stub("ArrowUp");
export const ArrowDown = stub("ArrowDown");
export const Search = stub("Search");
export const Loader = stub("Loader");
export const Users = stub("Users");
export const Lock = stub("Lock");
export const Mail = stub("Mail");
export const Bell = stub("Bell");
export const Shield = stub("Shield");
export const Palette = stub("Palette");
export const Lightbulb = stub("Lightbulb");
export const Rocket = stub("Rocket");
export const Heart = stub("Heart");
export const Paintbrush = stub("Paintbrush");
export const Brain = stub("Brain");
export const Globe = stub("Globe");
export const User = stub("User");
export const ImageIcon = stub("ImageIcon");
export const Link = stub("Link");
export const Check = stub("Check");
export const RotateCcw = stub("RotateCcw");
export const Play = stub("Play");
export const Pause = stub("Pause");
export const Pipette = stub("Pipette");
export const Home = stub("Home");
export const MessageCircle = stub("MessageCircle");
export const Inbox = stub("Inbox");
export const Pencil = stub("Pencil");
export const Scaling = stub("Scaling");
export const SkipForward = stub("SkipForward");
export const CornerDownRight = stub("CornerDownRight");
export const CornerDownLeft = stub("CornerDownLeft");
export const PanelLeft = stub("PanelLeft");
export const PanelRight = stub("PanelRight");
export const ChevronsUpDown = stub("ChevronsUpDown");
export const Ellipsis = stub("Ellipsis");
export const EllipsisVertical = stub("EllipsisVertical");
export const Calendar = stub("Calendar");
export const Folder = stub("Folder");
export const SlidersHorizontal = stub("SlidersHorizontal");
export const Info = stub("Info");
export const CircleAlert = stub("CircleAlert");
export const File = stub("File");
export const Minus = stub("Minus");
export const Maximize2 = stub("Maximize2");
export const Minimize2 = stub("Minimize2");
export const GripVertical = stub("GripVertical");
export const Tag = stub("Tag");

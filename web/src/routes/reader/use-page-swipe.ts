import { useCallback, useEffect, useRef } from "react";
import { animate, useMotionValue, type AnimationPlaybackControls } from "motion/react";

// ---------------------------------------------------------------------------
// Page strip physics (designed with the `feel` skill).
//
// Model: the strip is a 0.45-mass body on a 3.58 Hz spring with ζ 1.0.
//  - mass 0.45 ("light"): a page is paper; it keeps up with a fast finger.
//  - 3.58 Hz ("snappy"): a turn reads as immediate — rise 0.15 s.
//  - ζ 1.0 (critically damped, chosen over the 0.81 the words give):
//    pages must land exactly; any overshoot flashes the neighbouring page.
//    Settles (±2%) in 0.26 s.
// Release decision is projection, not a threshold: where the strip would
// coast under its free-flight friction (τ = m/c_f = 0.2 s, i.e. a 1000 px/s
// flick travels 200 px) decides the page. Past the first/last page the
// finger drives a wall: displacement is scaled by 0.35 (rubber band) and the
// same spring returns it.
// Framer/Motion export of the snap spring:
//   { type: "spring", mass: 0.45, stiffness: 227.7, damping: 20.25 }
// Reduced motion: MotionConfig("user") makes these jumps instant.
// ---------------------------------------------------------------------------

export const PAGE_SPRING = { type: "spring" as const, mass: 0.45, stiffness: 227.7, damping: 20.25 };
const COAST_TAU = 0.2; // s — free-flight friction time constant for projection
const WALL = 0.35; // rubber-band ratio beyond the ends
const TAP_SLOP = 6; // px a press may wander and still be a tap

interface Options {
  axis: "x" | "y";
  /** +1 when advancing moves content toward -axis (LTR, vertical); -1 for RTL. */
  forward: 1 | -1;
  canPrev: boolean;
  canNext: boolean;
  onCommit: (delta: 1 | -1) => void;
  onTap: (e: PointerEvent) => void;
}

/** Returns a motion value for the strip offset (px) and pointer handlers. */
export function usePageSwipe(opts: Options) {
  const offset = useMotionValue(0);
  const o = useRef(opts);
  o.current = opts;
  const anim = useRef<AnimationPlaybackControls | null>(null);
  const drag = useRef<{ id: number; start: number; origin: number; last: number; lastT: number; v: number; moved: boolean } | null>(null);
  const size = useRef(0);

  const pos = (e: PointerEvent | React.PointerEvent) => (o.current.axis === "x" ? e.clientX : e.clientY);

  /** Turn the page programmatically (keys, zones): same spring, carried velocity. */
  const turn = useCallback(
    (delta: 1 | -1) => {
      const { forward, canNext, canPrev, onCommit } = o.current;
      if ((delta === 1 && !canNext) || (delta === -1 && !canPrev)) {
        // Bump the wall: a small push the spring returns.
        anim.current?.stop();
        anim.current = animate(offset, 0, { ...PAGE_SPRING, velocity: -delta * forward * 900 });
        return;
      }
      // The new page mounts at 0; start the strip one page "behind" it so the
      // spring carries it home with the motion the turn implies.
      onCommit(delta);
      anim.current?.stop();
      offset.set(offset.get() + delta * forward * size.current);
      anim.current = animate(offset, 0, PAGE_SPRING);
    },
    [offset],
  );

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      if (e.button !== 0) return;
      const el = e.currentTarget;
      size.current = o.current.axis === "x" ? el.clientWidth : el.clientHeight;
      anim.current?.stop(); // interruptible: keep current offset and take over
      const p = pos(e);
      drag.current = { id: e.pointerId, start: p, origin: offset.get(), last: p, lastT: e.timeStamp, v: 0, moved: false };
      el.setPointerCapture(e.pointerId);
    },
    [offset],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      const d = drag.current;
      if (!d || d.id !== e.pointerId) return;
      const p = pos(e);
      const dt = Math.max(1, e.timeStamp - d.lastT) / 1000;
      // Light exponential smoothing on velocity; stale samples age out.
      d.v = 0.7 * ((p - d.last) / dt) + 0.3 * d.v;
      d.last = p;
      d.lastT = e.timeStamp;
      let x = d.origin + (p - d.start);
      if (Math.abs(p - d.start) > TAP_SLOP) d.moved = true;
      const { forward, canNext, canPrev } = o.current;
      // Dragging toward "next" means x moves by -forward.
      const towardNext = -x * forward > 0;
      if ((towardNext && !canNext) || (!towardNext && !canPrev)) x *= WALL;
      offset.set(x);
    },
    [offset],
  );

  const onPointerUp = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      const d = drag.current;
      if (!d || d.id !== e.pointerId) return;
      drag.current = null;
      if (!d.moved) {
        anim.current = animate(offset, 0, PAGE_SPRING);
        o.current.onTap(e.nativeEvent);
        return;
      }
      // 60 ms rule: a finger that stopped before lifting has no velocity.
      const v = e.timeStamp - d.lastT > 60 ? 0 : d.v;
      const x = offset.get();
      const projected = x + v * COAST_TAU;
      const half = size.current / 2;
      const { forward, canNext, canPrev, onCommit } = o.current;
      const step = -projected * forward > half ? 1 : projected * forward > half ? -1 : 0;
      if ((step === 1 && canNext) || (step === -1 && canPrev)) {
        onCommit(step as 1 | -1);
        // The committed page renders at 0; re-base the strip so the visual
        // position is continuous, then let the spring land it with release v.
        offset.set(x + step * forward * size.current);
        anim.current = animate(offset, 0, { ...PAGE_SPRING, velocity: v });
      } else {
        anim.current = animate(offset, 0, { ...PAGE_SPRING, velocity: v });
      }
    },
    [offset],
  );

  useEffect(() => () => anim.current?.stop(), []);

  return { offset, turn, handlers: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel: onPointerUp } };
}

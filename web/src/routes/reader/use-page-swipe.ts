import { useCallback, useEffect, useRef, type RefObject } from "react";
import { animate, useMotionValue, useReducedMotion, type AnimationPlaybackControls } from "motion/react";

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
// Reduced motion: offsets jump to rest (imperative animate() does not read
// MotionConfig, so the hook checks the preference itself).
// ---------------------------------------------------------------------------

export const PAGE_SPRING = { type: "spring" as const, mass: 0.45, stiffness: 227.7, damping: 20.25 };
const COAST_TAU = 0.2; // s — free-flight friction time constant for projection
const WALL = 0.35; // rubber-band ratio beyond the ends
const TAP_SLOP = 6; // px a press may wander and still be a tap

interface Options {
  stageRef: RefObject<HTMLElement | null>;
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
  const reduce = useReducedMotion() ?? false;
  const o = useRef(opts);
  o.current = opts;
  const anim = useRef<AnimationPlaybackControls | null>(null);
  const drag = useRef<{ id: number; start: number; origin: number; last: number; lastT: number; v: number; moved: boolean } | null>(null);

  const size = () => {
    const el = o.current.stageRef.current;
    return el ? (o.current.axis === "x" ? el.clientWidth : el.clientHeight) : 0;
  };
  const pos = (e: PointerEvent | React.PointerEvent) => (o.current.axis === "x" ? e.clientX : e.clientY);
  const clamp = (v: number) => {
    const s = size();
    return Math.max(-s, Math.min(s, v));
  };

  const settle = useCallback(
    (velocity = 0) => {
      anim.current?.stop();
      if (reduce) {
        offset.set(0);
        return;
      }
      anim.current = animate(offset, 0, { ...PAGE_SPRING, velocity });
    },
    [offset, reduce],
  );

  /** Re-base the strip after the view index moved by `delta`, keeping the
   *  picture continuous. Clamped to one page so rapid turns can't push every
   *  mounted slot off-screen; an active drag is re-based with it. */
  const rebase = useCallback(
    (delta: 1 | -1) => {
      const shift = delta * o.current.forward * size();
      const before = offset.get();
      const after = clamp(before + shift);
      offset.set(after);
      if (drag.current) drag.current.origin += after - before;
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [offset],
  );

  /** Turn the page programmatically (keys, zones): same spring. */
  const turn = useCallback(
    (delta: 1 | -1) => {
      const { forward, canNext, canPrev, onCommit } = o.current;
      if ((delta === 1 && !canNext) || (delta === -1 && !canPrev)) {
        settle(-delta * forward * 900); // bump the wall; the spring returns it
        return;
      }
      onCommit(delta);
      anim.current?.stop();
      rebase(delta);
      if (!drag.current) settle();
    },
    [rebase, settle],
  );

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      if (e.button !== 0) return;
      anim.current?.stop(); // interruptible: keep current offset and take over
      const p = pos(e);
      drag.current = { id: e.pointerId, start: p, origin: offset.get(), last: p, lastT: e.timeStamp, v: 0, moved: false };
      e.currentTarget.setPointerCapture(e.pointerId);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [offset],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      const d = drag.current;
      if (!d || d.id !== e.pointerId) return;
      const p = pos(e);
      const dt = Math.max(1, e.timeStamp - d.lastT) / 1000;
      d.v = 0.7 * ((p - d.last) / dt) + 0.3 * d.v; // light smoothing
      d.last = p;
      d.lastT = e.timeStamp;
      if (Math.abs(p - d.start) > TAP_SLOP) d.moved = true;
      let x = d.origin + (p - d.start);
      const { forward, canNext, canPrev } = o.current;
      const towardNext = -x * forward > 0; // dragging toward "next" moves x by -forward
      if ((towardNext && !canNext) || (!towardNext && !canPrev)) x *= WALL;
      offset.set(x);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [offset],
  );

  const onPointerUp = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      const d = drag.current;
      if (!d || d.id !== e.pointerId) return;
      drag.current = null;
      if (!d.moved) {
        settle();
        o.current.onTap(e.nativeEvent);
        return;
      }
      const v = e.timeStamp - d.lastT > 60 ? 0 : d.v; // 60 ms rule: a stopped finger has no velocity
      const projected = offset.get() + v * COAST_TAU;
      const half = size() / 2;
      const { forward, canNext, canPrev, onCommit } = o.current;
      const step = -projected * forward > half ? 1 : projected * forward > half ? -1 : 0;
      if ((step === 1 && canNext) || (step === -1 && canPrev)) {
        onCommit(step as 1 | -1);
        rebase(step as 1 | -1);
      }
      settle(v);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [offset, rebase, settle],
  );

  /** A cancelled pointer (the browser took the gesture, e.g. native scroll)
   *  never taps or turns: the strip just returns to rest. */
  const onPointerCancel = useCallback(
    (e: React.PointerEvent<HTMLElement>) => {
      if (drag.current?.id !== e.pointerId) return;
      drag.current = null;
      settle();
    },
    [settle],
  );

  useEffect(() => () => anim.current?.stop(), []);

  return { offset, turn, handlers: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel } };
}

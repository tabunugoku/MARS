// Every motion in the video is a damped spring (mass, stiffness, damping),
// never a duration with an easing curve. The numbers come from the feel
// skill's vocabulary (Feel.words) and are used as-is by Remotion's spring(),
// which solves the same equation. How long a motion takes follows from them.
import { spring } from "remotion";

export const FPS = 30;

export const FEEL = {
  // "heavy, smooth": windows and phones. 1.56 Hz, ζ 0.87, 0.4% overshoot,
  // settles in about 0.45 s. A screen is a heavy object that lands without
  // bouncing.
  window: { mass: 3, stiffness: 287.8, damping: 51.13 },
  // "light, snappy": a manga page. 3.19 Hz, ζ 0.72, 3.8% overshoot, settles
  // in about 0.3 s. Paper is light and lands with a small give.
  page: { mass: 0.45, stiffness: 181.2, damping: 13 },
  // "soft": captions and text. 1.4 Hz, ζ 1.0, no overshoot, about 0.66 s.
  soft: { mass: 1, stiffness: 77.4, damping: 17.59 },
  // "calm": the slow camera push. 1.1 Hz, ζ 1.1, about 1 s.
  calm: { mass: 1, stiffness: 47.8, damping: 15.21 },
  // "crisp": taps, chips and the selection in a control. 3.8 Hz, ζ 0.92,
  // 0.1% overshoot, about 0.2 s.
  crisp: { mass: 1, stiffness: 570.1, damping: 43.93 },
} as const;

export type Feel = keyof typeof FEEL;

/** 0 → 1 on the given spring, starting at `delay` frames. */
export function sp(frame: number, feel: Feel, delay = 0): number {
  return spring({ frame: frame - delay, fps: FPS, config: FEEL[feel] });
}

export const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

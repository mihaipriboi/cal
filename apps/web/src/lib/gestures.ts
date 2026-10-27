// Swipe detection for moving between calendar periods: two-finger touchpad
// swipes (wheel events) and finger swipes (touch pointers). Pure logic, so
// the thresholds can be tested without a browser.
//
// Direction follows the content, as in Google Calendar and natural
// scrolling: content moving left (deltaX > 0, or a finger moving left) goes
// to the next period, +1; moving right goes to the previous one, -1.

export type Step = -1 | 0 | 1;

export interface WheelInput {
  deltaX: number;
  deltaY: number;
  /** 0 = pixels, 1 = lines, 2 = pages (WheelEvent.deltaMode). */
  deltaMode: number;
  /** Event time in ms (Event.timeStamp). */
  timeStamp: number;
  ctrlKey?: boolean;
}

export interface WheelResult {
  /** The period to move by: fires once per gesture. */
  step: Step;
  /** The event belongs to a horizontal swipe, so its default (scrolling,
   *  the browser's back/forward swipe) should be prevented. */
  horizontal: boolean;
}

const LINE_PX = 16;
/** Movement before a gesture is classified as horizontal or vertical. */
const CLASSIFY_PX = 12;

/**
 * createWheelSwipe tracks touchpad gestures from wheel events. A gesture is a
 * run of events with gaps shorter than lockMs. It's horizontal once the
 * horizontal movement is more than 1.5× the vertical, and fires one step when
 * it has moved `threshold` px. The rest of the gesture, including the
 * touchpad's momentum, is absorbed, so one swipe is always one step. A
 * gesture classified as vertical stays vertical, so scrolling a time grid
 * diagonally never navigates.
 */
export function createWheelSwipe({ threshold = 60, lockMs = 250 }: { threshold?: number; lockMs?: number } = {}) {
  let sumX = 0;
  let sumY = 0;
  let last = -Infinity;
  let state: "idle" | "horizontal" | "vertical" | "fired" = "idle";

  return function onWheel(event: WheelInput, pageWidth = 800): WheelResult {
    // Pinch-zoom arrives as ctrl+wheel.
    if (event.ctrlKey) return { step: 0, horizontal: false };
    if (event.timeStamp - last > lockMs) {
      sumX = 0;
      sumY = 0;
      state = "idle";
    }
    last = event.timeStamp;
    if (state === "fired") return { step: 0, horizontal: true };
    if (state === "vertical") return { step: 0, horizontal: false };

    const scale = event.deltaMode === 1 ? LINE_PX : event.deltaMode === 2 ? pageWidth : 1;
    sumX += event.deltaX * scale;
    sumY += event.deltaY * scale;
    if (state === "idle" && Math.abs(sumX) + Math.abs(sumY) >= CLASSIFY_PX) {
      state = Math.abs(sumX) > 1.5 * Math.abs(sumY) ? "horizontal" : "vertical";
    }
    if (state !== "horizontal") return { step: 0, horizontal: false };
    if (Math.abs(sumX) >= threshold) {
      state = "fired";
      return { step: sumX > 0 ? 1 : -1, horizontal: true };
    }
    return { step: 0, horizontal: true };
  };
}

/** Width of the screen edges where Android's back gesture starts. */
export const EDGE_PX = 24;

/** A touch that starts at a screen edge belongs to the system's back gesture. */
export function touchStartAllowed(x: number, viewportWidth: number): boolean {
  return x >= EDGE_PX && x <= viewportWidth - EDGE_PX;
}

/** Movement before a touch is claimed as a swipe or left to scrolling. */
export const TOUCH_CLAIM_PX = 10;

/** After TOUCH_CLAIM_PX of movement: is the touch a horizontal swipe? */
export function touchIsHorizontal(dx: number, dy: number): boolean {
  return Math.abs(dx) > Math.abs(dy);
}

/**
 * touchSwipeDecision decides a finished finger swipe: it moves a period when
 * it covered a quarter of the width, or was a quick flick (over 0.35 px/ms
 * and 40 px), and was more horizontal than vertical.
 */
export function touchSwipeDecision({ dx, dy, dt, width }: { dx: number; dy: number; dt: number; width: number }): Step {
  if (Math.abs(dx) <= Math.abs(dy)) return 0;
  const far = Math.abs(dx) > 0.25 * width;
  const flick = Math.abs(dx) > 40 && Math.abs(dx) / Math.max(dt, 1) > 0.35;
  if (!far && !flick) return 0;
  return dx < 0 ? 1 : -1;
}

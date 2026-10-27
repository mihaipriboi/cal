import { useEffect, useRef, type RefObject } from "react";
import { swallowNextClick } from "../lib/clicks";
import { createWheelSwipe, touchIsHorizontal, touchStartAllowed, touchSwipeDecision, TOUCH_CLAIM_PX } from "../lib/gestures";

export interface SwipeHandlers {
  /** Move one period: +1 next, -1 previous. */
  onStep: (step: 1 | -1) => void;
  /** Swipes are ignored while this returns false (a dialog is open). */
  enabled: () => boolean;
  /** A finger swipe in progress: the view follows it by dx px. */
  follow: (dx: number) => void;
  /** A finger swipe ended without moving a period: the view snaps back. */
  settle: () => void;
}

// Elements a touch swipe never starts on: entries can be dragged to another
// day, and text fields need horizontal drags to select text.
const NO_SWIPE = '[draggable="true"], input, textarea, select, [contenteditable="true"]';

/**
 * useSwipeNavigation moves between periods on horizontal two-finger touchpad
 * swipes and finger swipes over `ref`. Vertical scrolling stays native: only
 * horizontal touchpad gestures have their default prevented, which also
 * keeps the browser's back/forward swipe from firing, and the container's
 * `touch-action: pan-y` leaves vertical panning to the browser.
 */
export function useSwipeNavigation(ref: RefObject<HTMLElement | null>, handlers: SwipeHandlers) {
  const latest = useRef(handlers);
  latest.current = handlers;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const wheel = createWheelSwipe();
    const onWheel = (event: WheelEvent) => {
      if (!latest.current.enabled()) return;
      const { step, horizontal } = wheel(event, el.clientWidth);
      if (horizontal) event.preventDefault();
      if (step) latest.current.onStep(step);
    };

    let touch: { id: number; x: number; y: number; t: number; claimed: boolean } | undefined;
    const down = (event: PointerEvent) => {
      touch = undefined;
      if (event.pointerType !== "touch" || !event.isPrimary || !latest.current.enabled()) return;
      if (!touchStartAllowed(event.clientX, window.innerWidth)) return;
      if ((event.target as Element).closest?.(NO_SWIPE)) return;
      touch = { id: event.pointerId, x: event.clientX, y: event.clientY, t: event.timeStamp, claimed: false };
    };
    const move = (event: PointerEvent) => {
      if (!touch || event.pointerId !== touch.id) return;
      const dx = event.clientX - touch.x;
      const dy = event.clientY - touch.y;
      if (!touch.claimed) {
        if (Math.abs(dx) < TOUCH_CLAIM_PX && Math.abs(dy) < TOUCH_CLAIM_PX) return;
        if (!touchIsHorizontal(dx, dy)) {
          touch = undefined; // a scroll: the browser has it
          return;
        }
        touch.claimed = true;
      }
      latest.current.follow(dx);
    };
    const up = (event: PointerEvent) => {
      if (!touch || event.pointerId !== touch.id) return;
      const start = touch;
      touch = undefined;
      if (!start.claimed) return;
      // The finger lifted over a day or an hour slot: that's not a tap.
      swallowNextClick();
      const step = touchSwipeDecision({
        dx: event.clientX - start.x,
        dy: event.clientY - start.y,
        dt: event.timeStamp - start.t,
        width: el.clientWidth,
      });
      if (step) latest.current.onStep(step);
      else latest.current.settle();
    };
    const cancel = (event: PointerEvent) => {
      if (!touch || event.pointerId !== touch.id) return;
      if (touch.claimed) latest.current.settle();
      touch = undefined;
    };

    el.addEventListener("wheel", onWheel, { passive: false });
    el.addEventListener("pointerdown", down);
    el.addEventListener("pointermove", move);
    el.addEventListener("pointerup", up);
    el.addEventListener("pointercancel", cancel);
    return () => {
      el.removeEventListener("wheel", onWheel);
      el.removeEventListener("pointerdown", down);
      el.removeEventListener("pointermove", move);
      el.removeEventListener("pointerup", up);
      el.removeEventListener("pointercancel", cancel);
    };
  }, [ref]);
}

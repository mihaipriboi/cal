import { describe, expect, it } from "vitest";
import { createWheelSwipe, touchIsHorizontal, touchStartAllowed, touchSwipeDecision, type WheelInput } from "./gestures";

/** Feeds a sequence of wheel events 16 ms apart and collects the steps. */
function run(events: Partial<WheelInput>[], start = 1000, swipe = createWheelSwipe()) {
  const steps: number[] = [];
  const horizontal: boolean[] = [];
  events.forEach((e, i) => {
    const out = swipe({ deltaX: 0, deltaY: 0, deltaMode: 0, timeStamp: start + i * 16, ...e });
    if (out.step) steps.push(out.step);
    horizontal.push(out.horizontal);
  });
  return { steps, horizontal, swipe };
}

const repeat = (n: number, e: Partial<WheelInput>) => Array.from({ length: n }, () => e);

describe("touchpad swipes", () => {
  it("fires once for a horizontal swipe, in the content's direction", () => {
    expect(run(repeat(10, { deltaX: 12 })).steps).toEqual([1]);
    expect(run(repeat(10, { deltaX: -12, deltaY: 2 })).steps).toEqual([-1]);
  });

  it("absorbs the momentum tail after firing", () => {
    // 6 events to fire, then 60 decaying momentum events with no gap.
    const tail = Array.from({ length: 60 }, (_, i) => ({ deltaX: Math.max(1, 30 - i / 2) }));
    const { steps, horizontal } = run([...repeat(6, { deltaX: 12 }), ...tail]);
    expect(steps).toEqual([1]);
    // The whole gesture keeps its default prevented (no back/forward swipe).
    expect(horizontal.every(Boolean)).toBe(true);
  });

  it("never fires for vertical scrolling", () => {
    const { steps, horizontal } = run(repeat(40, { deltaY: 20, deltaX: 3 }));
    expect(steps).toEqual([]);
    expect(horizontal.some(Boolean)).toBe(false);
  });

  it("stays vertical when a vertical scroll turns diagonal", () => {
    expect(run([...repeat(3, { deltaY: 10 }), ...repeat(20, { deltaX: 15, deltaY: 2 })]).steps).toEqual([]);
  });

  it("needs the threshold before firing", () => {
    expect(run(repeat(4, { deltaX: 14 })).steps).toEqual([]);
    expect(run(repeat(5, { deltaX: 14 })).steps).toEqual([1]);
  });

  it("normalises line and page deltas", () => {
    // Lines are 16 px: 4 lines reach 64 px.
    expect(run(repeat(4, { deltaX: 1, deltaMode: 1 })).steps).toEqual([1]);
    expect(run(repeat(3, { deltaX: 1, deltaMode: 1 })).steps).toEqual([]);
    expect(run([{ deltaX: -0.1, deltaMode: 2 }]).steps).toEqual([-1]);
  });

  it("fires again for a new gesture after a pause", () => {
    const { swipe } = run(repeat(10, { deltaX: 12 }));
    // Still inside the lock window: absorbed.
    expect(run(repeat(10, { deltaX: 12 }), 1000 + 9 * 16 + 200, swipe).steps).toEqual([]);
    // After a 300 ms pause: a new gesture.
    expect(run(repeat(10, { deltaX: -12 }), 1000 + 19 * 16 + 200 + 300, swipe).steps).toEqual([-1]);
  });

  it("ignores pinch-zoom", () => {
    expect(run(repeat(10, { deltaX: 20, ctrlKey: true })).steps).toEqual([]);
  });
});

describe("finger swipes", () => {
  const width = 390;
  it("moves on a quarter of the width, or a quick flick", () => {
    expect(touchSwipeDecision({ dx: -100, dy: 10, dt: 600, width })).toBe(1);
    expect(touchSwipeDecision({ dx: 100, dy: 10, dt: 600, width })).toBe(-1);
    // 60 px in 100 ms = 0.6 px/ms: a flick.
    expect(touchSwipeDecision({ dx: -60, dy: 5, dt: 100, width })).toBe(1);
    // 60 px in 400 ms: too slow and too short.
    expect(touchSwipeDecision({ dx: -60, dy: 5, dt: 400, width })).toBe(0);
    // Fast but under 40 px.
    expect(touchSwipeDecision({ dx: -35, dy: 0, dt: 30, width })).toBe(0);
  });

  it("ignores mostly vertical movement", () => {
    expect(touchSwipeDecision({ dx: -150, dy: 160, dt: 200, width })).toBe(0);
    expect(touchIsHorizontal(12, 4)).toBe(true);
    expect(touchIsHorizontal(6, 11)).toBe(false);
  });

  it("leaves touches at the screen edges to the system back gesture", () => {
    expect(touchStartAllowed(10, width)).toBe(false);
    expect(touchStartAllowed(width - 10, width)).toBe(false);
    expect(touchStartAllowed(24, width)).toBe(true);
    expect(touchStartAllowed(200, width)).toBe(true);
  });
});

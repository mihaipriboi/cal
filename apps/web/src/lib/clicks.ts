/** Stops the click the browser sends after a gesture that already acted
 *  (dismissing a popover, a swipe), so it doesn't also reach what's below. */
export function swallowNextClick() {
  const stop = (event: Event) => {
    event.stopPropagation();
    event.preventDefault();
  };
  window.addEventListener("click", stop, { capture: true, once: true });
  window.setTimeout(() => window.removeEventListener("click", stop, { capture: true }), 400);
}

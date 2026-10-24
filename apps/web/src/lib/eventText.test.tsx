import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { formatTime } from "./date";
import { countryName, descriptionToText, eventWhen, linkify, placeCard } from "./eventText";

const html = (text: string) => renderToStaticMarkup(<>{linkify(text)}</>);

describe("descriptionToText", () => {
  it("turns <br> and block tags into line breaks", () => {
    expect(descriptionToText("Line 1<br>Line 2<BR/>Line 3")).toBe("Line 1\nLine 2\nLine 3");
    expect(descriptionToText("<p>One</p>\n<p>Two</p>")).toBe("One\n\nTwo");
    expect(descriptionToText("<div>One</div><div>Two</div>")).toBe("One\nTwo");
    expect(descriptionToText("Agenda:<ul><li>a</li><li>b</li></ul>Bye")).toBe("Agenda:\n\n• a\n• b\n\nBye");
  });

  it("strips other tags and decodes entities", () => {
    expect(descriptionToText("<b>Bold</b> &amp; <i>it&#39;s</i> &lt;fine&gt;&nbsp;&#x263A;")).toBe("Bold & it's <fine> ☺");
  });

  it("reduces links to text that can be linkified again", () => {
    expect(descriptionToText('<a href="https://zoom.us/j/1">https://zoom.us/j/1</a>')).toBe("https://zoom.us/j/1");
    expect(descriptionToText('Join <a href="https://zoom.us/j/1">zoom.us/j/1</a>')).toBe("Join https://zoom.us/j/1");
    expect(descriptionToText('<a href="https://example.com/doc?a=1&amp;b=2">the doc</a>')).toBe(
      "the doc (https://example.com/doc?a=1&b=2)",
    );
    expect(descriptionToText('<a href="mailto:ana@example.com">ana@example.com</a>')).toBe("mailto:ana@example.com");
    const text = descriptionToText('See <a href="https://example.com">the site</a>.');
    expect(html(text)).toContain('<a href="https://example.com" target="_blank" rel="noreferrer">https://example.com</a>');
  });

  it("drops script and style contents and event-handler markup", () => {
    const out = descriptionToText('Hi<script>alert(1)</script><style>p{}</style><img src=x onerror="alert(2)">!');
    expect(out).toBe("Hi!");
    expect(descriptionToText("A<script>never closed")).toBe("A");
  });

  it("keeps the label of an unsafe link without linking it", () => {
    const out = descriptionToText('<a href="javascript:alert(1)">click</a> <a href="data:text/html,x">data</a>');
    expect(out).toBe("click data");
    expect(html(out)).not.toContain("<a");
  });

  it("leaves plain text alone", () => {
    expect(descriptionToText("if a<b and c>d\n\n\n\nthen  ")).toBe("if a<b and c>d\n\nthen");
    expect(descriptionToText(undefined)).toBe("");
  });
});

describe("linkify", () => {
  it("links http(s), mailto and bare emails", () => {
    const out = html("Docs https://example.com/a, mail mailto:x@example.com or ana@example.ro");
    expect(out).toContain('<a href="https://example.com/a" target="_blank" rel="noreferrer">https://example.com/a</a>,');
    expect(out).toContain('<a href="mailto:x@example.com"');
    expect(out).toContain('<a href="mailto:ana@example.ro"');
  });

  it("keeps trailing punctuation and unbalanced brackets out of the link", () => {
    expect(html("(see https://example.com/x).")).toBe(
      '(see <a href="https://example.com/x" target="_blank" rel="noreferrer">https://example.com/x</a>).',
    );
    expect(html("https://en.wikipedia.org/wiki/Foo_(bar)!")).toContain(">https://en.wikipedia.org/wiki/Foo_(bar)</a>!");
  });

  it("does not link other schemes", () => {
    expect(html("javascript:alert(1) data:text/html,x")).not.toContain("<a");
  });

  it("escapes text instead of rendering markup", () => {
    expect(html("<img src=x onerror=alert(1)>")).toBe("&lt;img src=x onerror=alert(1)&gt;");
  });
});

describe("eventWhen", () => {
  const today = "2026-10-24";

  it("shows an all-day day without the current year", () => {
    const when = eventWhen({ date: "2026-10-31" }, today);
    expect(when.day).toMatch(/October/);
    expect(when.day).not.toMatch(/2026/);
    expect(when.time).toBe("All day");
  });

  it("adds the year when it isn't this year", () => {
    expect(eventWhen({ date: "2027-01-01" }, today).day).toMatch(/2027/);
  });

  it("shows a timed range", () => {
    const when = eventWhen({ date: "2026-10-31", startTime: "09:00", endTime: "10:30" }, today);
    expect(when.time).toBe(`${formatTime("09:00")} – ${formatTime("10:30")}`);
    expect(eventWhen({ date: "2026-10-31", startTime: "09:00", endTime: "09:00" }, today).time).toBe(formatTime("09:00"));
  });

  it("shows one range line for multi-day events", () => {
    const allDay = eventWhen({ date: "2026-10-30", endDate: "2026-11-02" }, today);
    expect(allDay.day).toMatch(/Oct.*30.*–.*Nov.*2/);
    expect(allDay.time).toBe("All day");
    const timed = eventWhen({ date: "2026-10-24", endDate: "2026-10-25", startTime: "22:00", endTime: "02:00" }, today);
    expect(timed.day).toContain(formatTime("22:00"));
    expect(timed.day).toContain(formatTime("02:00"));
    expect(timed.time).toBeUndefined();
    expect(eventWhen({ date: "2026-12-30", endDate: "2027-01-02" }, today).day).toMatch(/2026.*2027/);
  });
});

describe("placeCard", () => {
  const viewport = { width: 1280, height: 800 };

  it("opens on the right of the item when it fits", () => {
    expect(placeCard({ top: 100, left: 200, right: 300 }, 240, viewport)).toEqual({ left: 308, top: 100 });
  });

  it("falls back to the left side, then to the screen edge", () => {
    expect(placeCard({ top: 100, left: 1000, right: 1100 }, 240, viewport)).toEqual({ left: 672, top: 100 });
    expect(placeCard({ top: 100, left: 100, right: 700 }, 240, { width: 720, height: 800 })).toEqual({ left: 8, top: 100 });
  });

  it("stays inside the bottom of the screen", () => {
    expect(placeCard({ top: 700, left: 200, right: 300 }, 240, viewport).top).toBe(552);
    expect(placeCard({ top: 10, left: 200, right: 300 }, 900, viewport).top).toBe(8);
  });
});

describe("countryName", () => {
  it("names a region code, or falls back to it", () => {
    expect(countryName("RO")).not.toBe("");
    expect(countryName("not a code")).toBe("not a code");
  });
});

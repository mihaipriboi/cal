import { describe, expect, it } from "vitest";
import { hasRemoteContent, MAIL_FRAME_SANDBOX, mailFrameDoc } from "./mailframe";

describe("mailFrameDoc", () => {
  it("blocks remote images unless allowed", () => {
    const blocked = mailFrameDoc("<p>hi</p>", false);
    expect(blocked).toContain(`default-src 'none'; img-src data: cid:;`);
    expect(blocked).not.toContain("https:");
    expect(mailFrameDoc("<p>hi</p>", true)).toContain("img-src data: cid: https:;");
  });

  it("never allows script", () => {
    const doc = mailFrameDoc("<script>alert(1)</script>", true);
    expect(doc).not.toMatch(/script-src/);
    expect(doc).toContain("default-src 'none'");
    expect(MAIL_FRAME_SANDBOX).not.toMatch(/allow-scripts|allow-same-origin/);
  });

  it("keeps the message markup intact inside the body", () => {
    expect(mailFrameDoc(`<b>x</b>`, false)).toContain("<body><b>x</b></body>");
  });
});

describe("hasRemoteContent", () => {
  it.each([
    [`<img src="https://t.example/p.gif">`, true],
    [`<img src='//t.example/p.gif'>`, true],
    [`<td background="http://x/y.png">`, true],
    [`<div style="background:url(https://x/y.png)">`, true],
    [`<img src="cid:logo@x">`, false],
    [`<img src="data:image/png;base64,AAA">`, false],
    [`<a href="https://example.com">link</a>`, false],
  ])("%s → %s", (html, want) => {
    expect(hasRemoteContent(html)).toBe(want);
  });
});

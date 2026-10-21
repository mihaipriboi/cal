import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { renderMarkdown, safeHref } from "./markdown";

const html = (src: string) => renderToStaticMarkup(<>{renderMarkdown(src)}</>);

describe("safeHref", () => {
  it("allows http(s), mailto and same-origin paths", () => {
    for (const ok of ["https://example.com", "http://x.y/z", "mailto:a@b.c", "/api/files/x.png", "/tasks"]) {
      expect(safeHref(ok), ok).toBe(true);
    }
  });

  it("rejects script-capable and off-origin forms", () => {
    for (const bad of ["javascript:alert(1)", " javascript:alert(1)", "JaVaScRiPt:x", "data:text/html,x", "vbscript:x", "intent://x", "//evil.example/x", "/\\evil.example/x", "file:///etc/passwd"]) {
      expect(safeHref(bad), bad).toBe(false);
    }
  });
});

describe("renderMarkdown links and images", () => {
  it("renders unsafe links as plain text", () => {
    const out = html("[click](javascript:alert(1))");
    expect(out).not.toContain("<a");
    expect(out).toContain("click");
  });

  it("keeps safe links", () => {
    expect(html("[site](https://example.com)")).toContain('<a href="https://example.com"');
  });

  it("does not load protocol-relative images", () => {
    expect(html("![x](//evil.example/p.png)")).not.toContain("<img");
    expect(html("![x](/\\evil.example/p.png)")).not.toContain("<img");
    expect(html("![x](https://example.com/p.png)")).toContain("<img");
  });
});

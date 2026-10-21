// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { CalApi } from "@cal/api-client";

// The web UI is served BY the server — a stored remote must never redirect
// its API calls. App shells (Capacitor http://localhost, Wails) legitimately
// carry one.
describe("CalApi remote server", () => {
  beforeEach(() => {
    localStorage.clear();
    delete (window as { Capacitor?: unknown }).Capacitor;
    delete (window as { runtime?: unknown }).runtime;
  });

  it("ignores and clears a stored cal:server on a plain http(s) page", () => {
    localStorage.setItem("cal:server", "https://other.example.com");
    const api = new CalApi();
    expect(api.remote).toBe("");
    expect(localStorage.getItem("cal:server")).toBeNull();
  });

  it("honors cal:server inside the Capacitor shell (Android serves https://localhost)", () => {
    (window as { Capacitor?: unknown }).Capacitor = { isNativePlatform: () => true };
    localStorage.setItem("cal:server", "https://cal.example.com");
    expect(new CalApi().remote).toBe("https://cal.example.com");
  });

  it("honors cal:server inside the Wails shell (injected window.runtime)", () => {
    (window as { runtime?: unknown }).runtime = {};
    localStorage.setItem("cal:server", "https://cal.example.com");
    expect(new CalApi().remote).toBe("https://cal.example.com");
  });
});

// The web UI authenticates by HttpOnly cookie: a session token in
// localStorage or in asset URLs would only be readable/leakable.
describe("CalApi session storage", () => {
  beforeEach(() => {
    localStorage.clear();
    delete (window as { Capacitor?: unknown }).Capacitor;
    delete (window as { runtime?: unknown }).runtime;
  });

  it("keeps no session token on a plain http(s) page", () => {
    localStorage.setItem("cal:session", "stale-token");
    const api = new CalApi();
    expect(localStorage.getItem("cal:session")).toBeNull();
    api.setSession("fresh-token");
    expect(localStorage.getItem("cal:session")).toBeNull();
    expect(api.assetUrl("/api/files/a.png")).toBe("/api/files/a.png");
  });

  it("stores the session and signs asset URLs inside the Capacitor shell", () => {
    (window as { Capacitor?: unknown }).Capacitor = { isNativePlatform: () => true };
    localStorage.setItem("cal:server", "https://cal.example.com");
    const api = new CalApi();
    api.setSession("tok");
    expect(localStorage.getItem("cal:session")).toBe("tok");
    expect(api.assetUrl("/api/files/a.png")).toBe("https://cal.example.com/api/files/a.png?session=tok");
  });
});

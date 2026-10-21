// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Entry, Settings, User } from "@cal/api-client";
import { cacheEntries, cacheSettings, cacheUser, clearCachedData, readCachedEntries, readCachedSettings, readCachedUser } from "./offline";

describe("offline cache", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.unstubAllGlobals());

  it("never stores tokens with the settings", () => {
    cacheSettings({ ...readCachedSettings(), theme: "dark", widgetToken: "w-secret", apiToken: "a-secret", peopleShareToken: "p-secret" } as Settings);
    const raw = localStorage.getItem("cal:settings") ?? "";
    expect(raw).not.toMatch(/secret/);
    expect(readCachedSettings().theme).toBe("dark");
  });

  it("logout clears the user, the entries snapshot and the API response cache", () => {
    const del = vi.fn(() => Promise.resolve(true));
    vi.stubGlobal("caches", { delete: del });
    cacheUser({ id: "u1", email: "a@b.c" } as User);
    cacheEntries([{ id: "e1", title: "private" } as Entry]);
    cacheSettings({ ...readCachedSettings(), theme: "dark" });

    clearCachedData();

    expect(readCachedUser()).toBeUndefined();
    expect(readCachedEntries()).toEqual([]);
    expect(readCachedSettings().theme).toBe("dark"); // theme survives for the login screen
    expect(del).toHaveBeenCalledWith("cal-api");
  });
});

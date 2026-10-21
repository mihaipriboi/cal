import type { Entry, Settings, User } from "@cal/api-client";

const prefix = "cal:";

export function readCachedEntries(): Entry[] {
  return read<Entry[]>("entries") ?? [];
}

export function cacheEntries(entries: Entry[]): void {
  write("entries", entries);
}

export function readCachedUser(): User | undefined {
  return read<User>("user");
}

export function cacheUser(user: User): void {
  write("user", user);
}

// clearCachedData forgets the signed-in user's data on logout: the cached
// user, the entries snapshot and the service worker's API response cache.
export function clearCachedData(): void {
  try {
    localStorage.removeItem(prefix + "user");
    localStorage.removeItem(prefix + "entries");
  } catch {
    // no storage
  }
  if (typeof caches !== "undefined") void caches.delete("cal-api").catch(() => undefined);
}

export function readCachedSettings(): Settings {
  return (
    read<Settings>("settings") ?? {
      country: "US",
      showHolidays: true,
      theme: "system",
      weekStart: "monday",
      accent: "green",
      timezone: "UTC",
      city: "",
      widgetToken: "",
      apiToken: "",
    }
  );
}

// Tokens stay out of localStorage: the server returns them with the settings
// on every load, and a cached copy would outlive logout.
export function cacheSettings(settings: Settings): void {
  write("settings", { ...settings, widgetToken: "", apiToken: "", peopleShareToken: undefined });
}

function read<T>(key: string): T | undefined {
  try {
    const raw = localStorage.getItem(prefix + key);
    return raw ? (JSON.parse(raw) as T) : undefined;
  } catch {
    return undefined;
  }
}

function write(key: string, value: unknown): void {
  try {
    localStorage.setItem(prefix + key, JSON.stringify(value));
  } catch {
    // no storage (tests, SSR, private mode)
  }
}

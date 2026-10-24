export type EntryType = "task" | "note" | "link" | "event";
export type Theme = "light" | "dark" | "system";
export type WeekStart = "monday" | "sunday";
export type Recur = "none" | "daily" | "weekly" | "monthly" | "yearly";
export type Accent = "green" | "blue" | "violet" | "amber" | "rose";

export interface User {
  id: string;
  email: string;
  isAdmin?: boolean;
  createdAt?: string;
}

/** Public auth-panel state: whether the instance has accounts and accepts new ones. */
export interface AuthConfig {
  hasUsers: boolean;
  registrationOpen: boolean;
}

/** Admin instance config (GET/PUT /admin/config). */
export interface AdminConfig {
  allowRegistration: boolean;
}

export interface ShoppingList {
  id: string;
  name: string;
  icon: string;
  position: number;
  /** Total items on the list. */
  items: number;
  /** Unchecked items — what still needs buying. */
  open: number;
  createdAt: string;
}

export interface ShoppingSection {
  id: string;
  listId: string;
  name: string;
  position: number;
}

export interface ShoppingItem {
  id: string;
  listId: string;
  sectionId?: string;
  name: string;
  note: string;
  quantity: string;
  checked: boolean;
  /** "Can't find it" flag from the store. */
  uncertain: boolean;
  position: number;
  createdAt: string;
  checkedAt?: string;
}

/** Patch for an item; sectionId "" moves it to the unsectioned block. */
export interface ShoppingItemPatch {
  name?: string;
  note?: string;
  quantity?: string;
  sectionId?: string;
  checked?: boolean;
  uncertain?: boolean;
  position?: number;
}

export interface ShoppingSuggestion {
  name: string;
  section?: string;
}

export interface Entry {
  id: string;
  title: string;
  content?: string;
  type: EntryType;
  linkUrl?: string;
  /** YYYY-MM-DD; absent only on undated tasks (no time, repeat or reminder). */
  date?: string;
  /** HH:MM 24h; absent means all-day / untimed */
  startTime?: string;
  endTime?: string;
  completed: boolean;
  pinned: boolean;
  watched?: boolean;
  linkImage?: string;
  linkDesc?: string;
  linkFavicon?: string;
  linkVideoId?: string;
  /** Set once enrichment ran (fields may still be empty if unfurl failed); absent = pending. */
  linkMetaAt?: string;
  color: string;
  boardId?: string;
  columnId?: string;
  position?: number;
  tags: string[];
  recur: Recur;
  /** Minutes before startTime to fire a reminder; absent = none. */
  remind?: number;
  accountId?: string;
  workspaceId?: string;
  blockedBy?: string;
  createdAt: string;
}

export interface EntryInput {
  title: string;
  content?: string;
  type: EntryType;
  linkUrl?: string;
  /** Omit for an undated task; required for every other type. */
  date?: string;
  startTime?: string;
  endTime?: string;
  color?: string;
  tags?: string[];
  recur?: Recur;
  remind?: number | null;
  accountId?: string;
  boardId?: string;
  columnId?: string;
  workspaceId?: string;
  blockedBy?: string;
}

/** `date: null` (or "") makes a task undated. */
export type EntryPatch = Partial<Omit<EntryInput, "date"> & { completed: boolean; pinned: boolean; watched: boolean }> & {
  date?: string | null;
};

export interface Revision {
  id: string;
  entryId: string;
  title: string;
  content: string;
  type: EntryType;
  date?: string;
  startTime?: string;
  endTime?: string;
  completed: boolean;
  color: string;
  tags: string[];
  recur: Recur;
  remind?: number;
  savedAt: string;
}

export interface Settings {
  country: string;
  showHolidays: boolean;
  theme: Theme;
  weekStart: WeekStart;
  accent: Accent;
  timezone: string;
  city: string;
  quotaMb?: number;
  digestTime?: string;
  defaultRate?: number;
  /** A GitHub token is stored; the token itself is write-only. */
  githubConnected?: boolean;
  /** Completing a card closes its linked GitHub issue (off by default). */
  githubCloseIssues?: boolean;
  /** Today's agenda for widgets. */
  widgetToken: string;
  /** MCP: full read/write access. */
  apiToken: string;
  /** POST /api/intake: create tasks only. */
  intakeToken?: string;
  /** GET /api/feed.ics: read-only calendar export. */
  feedToken?: string;
  /** Feature modules on/off; absent key = enabled. */
  modules?: Record<string, boolean>;
  /** Calendar view fresh devices open on; per-device last-used view still wins. */
  defaultView?: "month" | "week" | "day";
  /** Active workspace id; undefined/"none"/"" semantics handled client-side. */
  activeWorkspace?: string;
  /** Default two-letter country for nameday lookups in the person editor. */
  namedayCountry?: string;
  /** Base URL of the user's Invidious instance for YouTube search ("" = off). */
  invidiousUrl?: string;
  /** Public people-list share token ("" = sharing off). */
  peopleShareToken?: string;
  /** Quick-added tasks with no day in the text get today instead of staying undated. */
  quickAddToday?: boolean;
  /** Tasks page lists the No date group first instead of last. */
  undatedFirst?: boolean;
}

/** One YouTube search hit from the user's Invidious instance. */
export interface YtResult {
  videoId: string;
  title: string;
  author: string;
  url: string;
  thumbnail: string;
  seconds: number;
  views: number;
}

export interface WeekReview {
  from: string;
  to: string;
  tasksDone: number;
  tasksSlipped: number;
  notesWritten: number;
  streak: number;
  busiestDay: string;
  busiestCount: number;
  perDay: Record<string, number>;
}

export interface Habit {
  id: string;
  title: string;
  recur: string;
  streak: number;
  lastDone?: string;
}

export interface RestorePreview {
  dryRun: boolean;
  entries: { total: number; new: number; existing: number; invalid: number };
  people: { total: number; new: number; existing: number; invalid: number };
  links: { total: number; new: number; existing: number; orphaned: number };
  timeline: { total: number; new: number; existing: number; orphaned: number };
  files: { total: number; new: number; existing: number; noBinary: number; invalid: number };
}

export interface Webhook {
  id: string;
  url: string;
  secret: string;
  createdAt: string;
}

export interface Unfurl {
  title: string;
  favicon: string;
  description: string;
}

export interface SessionInfo {
  id: string;
  userAgent?: string;
  createdAt: string;
  lastSeen?: string;
  current: boolean;
}

export interface CaldavAccount {
  id: string;
  name: string;
  url: string;
  username: string;
  color: string;
  lastSynced?: string;
}

/** A saved CardDAV addressbook (contact birthdays / people import). */
export interface CarddavAccount {
  id: string;
  name: string;
  url: string;
  username: string;
  lastSynced?: string;
}

export interface FileRec {
  id: string;
  name: string;
  origName: string;
  size: number;
  mime: string;
  shareToken?: string;
  total: number;
  done: number;
  createdAt: string;
  tags: string[];
  workspaceId?: string;
  /** Set when the file is attached to a person profile. */
  personId?: string;
}

export interface Feed {
  id: string;
  name: string;
  url: string;
  color: string;
  kind: string; // "calendar" | "links" — links feeds create bookmark entries
  fetchedAt?: string;
}

export interface FeedEvent {
  id: string;
  feedId: string;
  feedName: string;
  title: string;
  date: string;
  /** Last day of an event that ends on a later day than `date`. */
  endDate?: string;
  startTime?: string;
  endTime?: string;
  color: string;
  location?: string;
  url?: string;
  image?: string;
  details?: string;
}

export interface Holiday {
  id: string;
  name: string;
  date: string;
  country: string;
}

/** One named yearly date on a person — anniversary, nameday, "first met".
 *  Recurs on month+day; the year only feeds "turns N" displays. */
export interface PersonDate {
  label: string;
  date: string; // YYYY-MM-DD
  /** Days ahead to push a reminder; absent = no reminder. */
  remindDays?: number;
}

/** One custom key/value pair on a person profile. */
export interface PersonField {
  key: string;
  value: string;
}

/** One social/web link on a person profile. */
export interface PersonLink {
  platform: string;
  url: string;
}

/** Public people-page row — private fields never leave the server. */
export interface SharedPerson {
  name: string;
  nickname?: string;
  relation?: string;
  color?: string;
  birthday?: string;
  dates: PersonDate[];
}

export interface Person {
  id: string;
  name: string;
  /** Free-ish text — family | partner | friend | colleague | acquaintance by convention. */
  relation: string;
  birthday?: string;
  dates: PersonDate[];
  notes: string;
  color: string;
  workspaceId?: string;
  createdAt: string;
  nickname?: string;
  /** files.name of the avatar image. */
  avatar?: string;
  phone?: string;
  email?: string;
  address?: string;
  giftIdeas?: string;
  interests?: string;
  isFavorite: boolean;
  fields: PersonField[];
  links: PersonLink[];
  tags: string[];
  /** Days ahead to remind about the birthday; absent = off. */
  birthdayRemind?: number;
}

export interface PersonInput {
  name: string;
  relation?: string;
  /** YYYY-MM-DD or "" to clear. */
  birthday?: string;
  dates?: PersonDate[];
  notes?: string;
  color?: string;
  workspaceId?: string;
  nickname?: string;
  avatar?: string;
  phone?: string;
  email?: string;
  address?: string;
  giftIdeas?: string;
  interests?: string;
  isFavorite?: boolean;
  fields?: PersonField[];
  links?: PersonLink[];
  tags?: string[];
  birthdayRemind?: number | null;
}

/** Directed person→person edge, resolved for display around personId. */
export interface PersonRelation {
  id: string;
  kind: string; // parent|child|sibling|partner|friend|coworker|mentor
  personId: string;
  otherId: string;
  otherName?: string;
  outgoing: boolean;
  createdAt: string;
}

export interface TimelineItem {
  id: string;
  personId: string;
  type: string; // met|gift|trip|achievement|memory|note
  title: string;
  body?: string;
  occurredOn?: string;
  createdAt: string;
}

/** One matching (country, dates) nameday search result. */
export interface NamedayResult {
  country: string;
  dates: { day: number; month: number; name: string }[];
}

/** Public holiday as returned by the date.nager.at browse endpoint. */
export interface NagerHoliday {
  date: string;
  name: string;
  countryCode: string;
  nationalHoliday: boolean;
  subdivisionCodes?: string[];
  holidayTypes?: string[];
}

export interface Country {
  code: string;
  name: string;
}

export interface AuthRequest {
  email: string;
  password: string;
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

export class CalApi {
  // Native apps (Capacitor) can't use cookie auth cross-origin — they store
  // a server URL + Bearer session. Same-origin web uses neither.
  private server = "";
  private session = "";
  // Same-origin web authenticates by the HttpOnly cookie alone; keeping the
  // token in script-readable storage or in URLs would only expose it.
  private cookieOnly = false;

  constructor(private readonly baseUrl = "/api") {
    try {
      // A page served over http(s) IS the server — a remote URL only makes
      // sense inside app shells (Wails, Capacitor) where the shell and the
      // API live on different hosts. A stale stored value on the web UI would
      // silently send its calls to another server (and fail on CORS anyway),
      // so drop it. Android webviews run on http(s)://localhost too — detect
      // the shell, not the scheme.
      const proto = typeof location !== "undefined" ? location.protocol : "";
      const win = typeof window !== "undefined"
        ? (window as { Capacitor?: { isNativePlatform?: () => boolean }; runtime?: unknown })
        : undefined;
      const appShell =
        proto === "wails:" || proto === "capacitor:" || proto === "ionic:" ||
        win?.Capacitor?.isNativePlatform?.() === true || win?.runtime !== undefined;
      const servedByServer = /^https?:$/.test(proto) && !appShell;
      this.server = servedByServer ? "" : (localStorage.getItem("cal:server") ?? "");
      if (servedByServer) localStorage.removeItem("cal:server");
      this.cookieOnly = servedByServer;
      if (servedByServer) localStorage.removeItem("cal:session");
      else this.session = localStorage.getItem("cal:session") ?? "";
    } catch {
      // storage unavailable
    }
  }

  get remote(): string {
    return this.server;
  }

  setServer(url: string) {
    this.server = url.replace(/\/+$/, "");
    try {
      if (this.server) localStorage.setItem("cal:server", this.server);
      else localStorage.removeItem("cal:server");
    } catch { /* ignore */ }
  }

  setSession(token: string) {
    this.session = this.cookieOnly ? "" : token;
    try {
      if (this.session) localStorage.setItem("cal:session", this.session);
      else localStorage.removeItem("cal:session");
    } catch { /* ignore */ }
  }

  // assetUrl resolves a /api/… path for <img src> and friends — absolute on
  // native, with the session as a query param since media tags send no headers.
  assetUrl(path: string): string {
    const base = this.server ? `${this.server}${path}` : path;
    return this.session ? `${base}${path.includes("?") ? "&" : "?"}session=${encodeURIComponent(this.session)}` : base;
  }

  private get root(): string {
    return this.server ? `${this.server}${this.baseUrl}` : this.baseUrl;
  }

  async register(input: AuthRequest): Promise<{ user: User; session: string }> {
    const out = await this.request<{ user: User; session: string }>("/auth/register", { method: "POST", body: input });
    if (out.session) this.setSession(out.session);
    return out;
  }

  async login(input: AuthRequest): Promise<{ user: User; session: string }> {
    const out = await this.request<{ user: User; session: string }>("/auth/login", { method: "POST", body: input });
    if (out.session) this.setSession(out.session);
    return out;
  }

  async logout(): Promise<void> {
    await this.request<void>("/auth/logout", { method: "POST" });
  }

  async me(): Promise<User> {
    return this.request<User>("/me");
  }

  /** Public — no session needed; the auth panel uses it before login. */
  async authConfig(): Promise<AuthConfig> {
    return this.request<AuthConfig>("/auth/config");
  }

  async adminUsers(): Promise<User[]> {
    return this.request<User[]>("/admin/users");
  }

  async adminSetUserAdmin(id: string, isAdmin: boolean): Promise<void> {
    await this.request(`/admin/users/${id}`, { method: "PATCH", body: { isAdmin } });
  }

  async adminDeleteUser(id: string): Promise<void> {
    await this.request(`/admin/users/${id}`, { method: "DELETE" });
  }

  async adminConfig(): Promise<AdminConfig> {
    return this.request<AdminConfig>("/admin/config");
  }

  async adminUpdateConfig(input: AdminConfig): Promise<AdminConfig> {
    return this.request<AdminConfig>("/admin/config", { method: "PUT", body: input });
  }

  // ---------- Shopping lists ----------

  async shoppingLists(): Promise<ShoppingList[]> {
    return this.request<ShoppingList[]>("/shopping/lists");
  }

  async shoppingCreateList(input: { name: string; icon?: string }): Promise<ShoppingList> {
    return this.request<ShoppingList>("/shopping/lists", { method: "POST", body: input });
  }

  async shoppingList(id: string): Promise<{ sections: ShoppingSection[]; items: ShoppingItem[] }> {
    return this.request(`/shopping/lists/${id}`);
  }

  async shoppingUpdateList(id: string, input: { name: string; icon?: string }): Promise<ShoppingList> {
    return this.request<ShoppingList>(`/shopping/lists/${id}`, { method: "PATCH", body: input });
  }

  async shoppingDeleteList(id: string): Promise<void> {
    await this.request(`/shopping/lists/${id}`, { method: "DELETE" });
  }

  async shoppingCreateSection(listId: string, name: string): Promise<ShoppingSection> {
    return this.request<ShoppingSection>(`/shopping/lists/${listId}/sections`, { method: "POST", body: { name } });
  }

  async shoppingUpdateSection(id: string, name: string): Promise<void> {
    await this.request(`/shopping/sections/${id}`, { method: "PATCH", body: { name } });
  }

  async shoppingDeleteSection(id: string): Promise<void> {
    await this.request(`/shopping/sections/${id}`, { method: "DELETE" });
  }

  async shoppingCheckSection(id: string, checked: boolean): Promise<void> {
    await this.request(`/shopping/sections/${id}/check`, { method: "POST", body: { checked } });
  }

  async shoppingClearPurchased(listId: string): Promise<{ removed: number }> {
    return this.request<{ removed: number }>(`/shopping/lists/${listId}/clear`, { method: "POST", body: {} });
  }

  async shoppingCreateItem(listId: string, input: { name: string; note?: string; quantity?: string; sectionId?: string }): Promise<ShoppingItem> {
    return this.request<ShoppingItem>(`/shopping/lists/${listId}/items`, { method: "POST", body: input });
  }

  async shoppingUpdateItem(id: string, patch: ShoppingItemPatch): Promise<ShoppingItem> {
    return this.request<ShoppingItem>(`/shopping/items/${id}`, { method: "PATCH", body: patch });
  }

  async shoppingDeleteItem(id: string): Promise<void> {
    await this.request(`/shopping/items/${id}`, { method: "DELETE" });
  }

  async shoppingSuggest(q: string): Promise<ShoppingSuggestion[]> {
    return this.request<ShoppingSuggestion[]>(`/shopping/suggest?q=${encodeURIComponent(q)}`);
  }

  /** `undated` adds undated tasks to a ranged query (an unranged one has them already). */
  async entries(params: { from?: string; to?: string; q?: string; workspace?: string; undated?: boolean } = {}): Promise<Entry[]> {
    const search = new URLSearchParams();
    if (params.from) search.set("from", params.from);
    if (params.to) search.set("to", params.to);
    if (params.q) search.set("q", params.q);
    if (params.workspace) search.set("workspace", params.workspace);
    if (params.undated) search.set("undated", "1");
    const suffix = search.size ? `?${search.toString()}` : "";
    return this.request<Entry[]>(`/entries${suffix}`);
  }

  async refreshLink(id: string): Promise<void> {
    await this.request(`/entries/${id}/refresh-link`, { method: "POST" });
  }

  async createEntry(input: EntryInput): Promise<Entry> {
    return this.request<Entry>("/entries", { method: "POST", body: input });
  }

  async updateEntry(id: string, patch: EntryPatch): Promise<Entry> {
    return this.request<Entry>(`/entries/${id}`, { method: "PATCH", body: patch });
  }

  async deleteEntry(id: string): Promise<void> {
    await this.request<void>(`/entries/${id}`, { method: "DELETE" });
  }

  async settings(): Promise<Settings> {
    return this.request<Settings>("/settings");
  }

  async updateSettings(settings: Settings): Promise<Settings> {
    return this.request<Settings>("/settings", { method: "PUT", body: settings });
  }

  async holidays(country: string, year: number): Promise<Holiday[]> {
    return this.request<Holiday[]>(`/holidays?country=${encodeURIComponent(country)}&year=${year}`);
  }

  async countries(): Promise<Country[]> {
    return this.request<Country[]>("/holidays/countries");
  }

  async feeds(): Promise<Feed[]> {
    return this.request<Feed[]>("/feeds");
  }

  async createFeed(input: { name: string; url: string; color?: string; kind?: string }): Promise<Feed> {
    return this.request<Feed>("/feeds", { method: "POST", body: input });
  }

  /** Renames and/or recolours a feed; its URL can't be changed. */
  async updateFeed(id: string, patch: { name?: string; color?: string }): Promise<Feed> {
    return this.request<Feed>(`/feeds/${id}`, { method: "PATCH", body: patch });
  }

  async deleteFeed(id: string): Promise<void> {
    await this.request<void>(`/feeds/${id}`, { method: "DELETE" });
  }

  async refreshFeed(id: string): Promise<void> {
    await this.request<void>(`/feeds/${id}/refresh`, { method: "POST" });
  }

  async feedEvents(params: { from: string; to: string }): Promise<FeedEvent[]> {
    const search = new URLSearchParams({ from: params.from, to: params.to });
    return this.request<FeedEvent[]>(`/feed-events?${search.toString()}`);
  }

  async weeklyReview(): Promise<WeekReview> {
    return this.request<WeekReview>("/review/week");
  }

  async habits(): Promise<Habit[]> {
    return this.request<Habit[]>("/habits");
  }

  async unfurl(url: string): Promise<Unfurl> {
    return this.request<Unfurl>(`/unfurl?url=${encodeURIComponent(url)}`);
  }

  async webhooks(): Promise<Webhook[]> {
    return this.request("/webhooks");
  }

  async addWebhook(url: string): Promise<Webhook> {
    return this.request("/webhooks", { method: "POST", body: { url } });
  }

  async deleteWebhook(id: string): Promise<void> {
    return this.request(`/webhooks/${id}`, { method: "DELETE" });
  }

  async discoverCaldav(input: { url: string; username: string; password: string }): Promise<{ href: string; name: string }[]> {
    return this.request("/caldav/discover", { method: "POST", body: input });
  }

  async connectCarddav(input: { name?: string; url: string; username: string; password: string }): Promise<{ imported: number; found: number }> {
    return this.request("/carddav", { method: "POST", body: input });
  }

  async googleStatus(): Promise<{ connected: boolean }> {
    return this.request("/google/status");
  }

  async googleConnect(): Promise<{ url: string }> {
    return this.request("/google/connect");
  }

  async googleSync(): Promise<void> {
    return this.request("/google/sync", { method: "POST" });
  }

  async googleDisconnect(): Promise<void> {
    return this.request("/google", { method: "DELETE" });
  }

  async storage(): Promise<{ usedBytes: number; quotaBytes: number }> {
    return this.request("/storage");
  }

  async files(): Promise<FileRec[]> {
    return this.request("/files");
  }

  async deleteFile(id: string): Promise<void> {
    return this.request(`/files/${id}`, { method: "DELETE" });
  }

  async shareFile(id: string, on: boolean): Promise<{ shareToken: string | null }> {
    return this.request(`/files/${id}/share`, { method: "POST", body: { on } });
  }

  async boards(): Promise<Board[]> {
    return this.request("/boards");
  }

  async createBoard(name: string, color?: string, template?: string): Promise<Board> {
    return this.request("/boards", { method: "POST", body: { name, color, template } });
  }

  async deleteBoard(id: string): Promise<void> {
    return this.request(`/boards/${id}`, { method: "DELETE" });
  }

  async boardView(id: string): Promise<{ columns: BoardColumn[]; cards: Entry[] }> {
    return this.request(`/boards/${id}/view`);
  }

  async createColumn(boardId: string, name: string): Promise<BoardColumn> {
    return this.request(`/boards/${boardId}/columns`, { method: "POST", body: { name } });
  }

  async updateColumn(id: string, patch: { name?: string; wipLimit?: number }): Promise<void> {
    return this.request(`/columns/${id}`, { method: "PATCH", body: patch });
  }

  async deleteColumn(id: string): Promise<void> {
    return this.request(`/columns/${id}`, { method: "DELETE" });
  }

  async moveCard(entryId: string, columnId: string | null, position: number): Promise<void> {
    return this.request(`/cards/${entryId}/move`, { method: "POST", body: { columnId, position } });
  }

  async startTimer(opts: { entryId?: string; note?: string; planned?: number; billable?: boolean; rate?: number; projectId?: string } = {}): Promise<TimeEntry> {
    return this.request("/timer/start", { method: "POST", body: opts });
  }
  async stopTimer(): Promise<TimeEntry> {
    return this.request("/timer/stop", { method: "POST" });
  }
  async currentTimer(): Promise<TimeEntry | null> {
    const r = await fetch(`${this.root}/timer/current`, { credentials: "include", headers: this.headers(false) });
    return r.status === 204 ? null : r.json();
  }
  async timeSummary(): Promise<TimeSummary> {
    return this.request("/time/summary");
  }
  async entryActivity(id: string): Promise<ActivityItem[]> {
    return this.request(`/entries/${id}/activity`);
  }
  async trash(): Promise<Entry[]> {
    return this.request("/trash");
  }
  async restoreEntry(id: string): Promise<void> {
    return this.request(`/trash/${id}/restore`, { method: "POST", body: "{}" });
  }
  async purgeEntry(id: string): Promise<void> {
    return this.request(`/trash/${id}`, { method: "DELETE" });
  }
  async updateBoard(id: string, patch: { description?: string; targetDate?: string }): Promise<void> {
    return this.request(`/boards/${id}`, { method: "PATCH", body: patch });
  }
  async shareBoard(id: string, on: boolean, edit = false): Promise<{ shareToken: string; edit: boolean }> {
    return this.request(`/boards/${id}/share`, { method: "POST", body: { on, edit } });
  }
  async sharedBoard(token: string): Promise<{ name: string; description: string; targetDate?: string; edit: boolean; columns: { id: string; name: string }[]; cards: { id: string; columnId?: string; title: string; completed: boolean; date: string; tags: string[] }[] }> {
    const r = await fetch(`${this.root}/shared/boards/${token}`, { headers: this.headers(false) });
    if (!r.ok) throw new Error("not found");
    return r.json();
  }
  async sharedBoardMove(token: string, cardId: string, columnId: string, position: number): Promise<void> {
    const r = await fetch(`${this.root}/shared/boards/${token}/cards/${cardId}/move`, {
      method: "POST",
      headers: this.headers(true),
      body: JSON.stringify({ columnId, position }),
    });
    if (!r.ok) throw new Error(await r.text());
  }
  async sharePeople(on: boolean): Promise<{ shareToken: string | null }> {
    return this.request(`/people/share`, { method: "POST", body: { on } });
  }
  async sharedPeople(token: string): Promise<{ people: SharedPerson[] }> {
    const r = await fetch(`${this.root}/shared/people/${token}`, { headers: this.headers(false) });
    if (!r.ok) throw new Error("not found");
    return r.json();
  }
  async tags(): Promise<Record<string, number>> {
    return this.request("/tags");
  }
  async timeLog(from?: string, to?: string): Promise<TimeEntry[]> {
    const q = from && to ? `?from=${from}&to=${to}` : "";
    return this.request(`/time/log${q}`);
  }
  async deleteTimeEntry(id: string): Promise<void> {
    return this.request(`/time/log/${id}`, { method: "DELETE" });
  }

  async createTimeEntry(input: TimeEntryInput): Promise<TimeEntry> {
    return this.request("/time/log", { method: "POST", body: input });
  }

  async updateTimeEntry(id: string, patch: TimeEntryPatch): Promise<TimeEntry> {
    return this.request(`/time/log/${id}`, { method: "PATCH", body: patch });
  }

  async dashboard(workspace?: string): Promise<DashboardStats> {
    const q = workspace ? `?workspace=${encodeURIComponent(workspace)}` : "";
    return this.request(`/dashboard${q}`);
  }

  async workspaces(): Promise<Workspace[]> {
    return this.request("/workspaces");
  }
  async createWorkspace(input: { name: string; color?: string; icon?: string }): Promise<Workspace> {
    return this.request("/workspaces", { method: "POST", body: input });
  }
  async updateWorkspace(id: string, patch: { name?: string; color?: string; icon?: string; position?: number }): Promise<void> {
    return this.request(`/workspaces/${id}`, { method: "PATCH", body: patch });
  }
  async deleteWorkspace(id: string): Promise<void> {
    return this.request(`/workspaces/${id}`, { method: "DELETE" });
  }

  async updateFile(id: string, patch: { tags?: string[]; workspaceId?: string }): Promise<FileRec> {
    return this.request(`/files/${id}`, { method: "PATCH", body: patch });
  }

  async people(): Promise<Person[]> {
    return this.request("/people");
  }
  async person(id: string): Promise<Person> {
    return this.request(`/people/${id}`);
  }
  async createPerson(input: PersonInput): Promise<Person> {
    return this.request("/people", { method: "POST", body: input });
  }
  async updatePerson(id: string, input: PersonInput): Promise<Person> {
    return this.request(`/people/${id}`, { method: "PATCH", body: input });
  }
  async deletePerson(id: string): Promise<void> {
    await this.request(`/people/${id}`, { method: "DELETE" });
  }

  async personRelations(id: string): Promise<PersonRelation[]> {
    return this.request(`/people/${id}/relations`);
  }
  /** Every link in the account — the family-tree graph. */
  async allPersonRelations(): Promise<PersonRelation[]> {
    return this.request("/people/relations");
  }
  async linkPersons(personId: string, toId: string, kind: string): Promise<PersonRelation> {
    return this.request(`/people/${personId}/relations`, { method: "POST", body: { toId, kind } });
  }
  async unlinkPersons(linkId: string): Promise<void> {
    await this.request(`/people/relations/${linkId}`, { method: "DELETE" });
  }

  async personTimeline(id: string): Promise<TimelineItem[]> {
    return this.request(`/people/${id}/timeline`);
  }
  async createTimelineItem(personId: string, input: { type: string; title: string; body?: string; occurredOn?: string }): Promise<TimelineItem> {
    return this.request(`/people/${personId}/timeline`, { method: "POST", body: input });
  }
  async updateTimelineItem(personId: string, itemId: string, input: { type: string; title: string; body?: string; occurredOn?: string }): Promise<TimelineItem> {
    return this.request(`/people/${personId}/timeline/${itemId}`, { method: "PATCH", body: input });
  }
  async deleteTimelineItem(personId: string, itemId: string): Promise<void> {
    await this.request(`/people/${personId}/timeline/${itemId}`, { method: "DELETE" });
  }

  async personFiles(id: string): Promise<FileRec[]> {
    return this.request(`/people/${id}/files`);
  }

  async searchNamedays(name: string, country?: string): Promise<NamedayResult[]> {
    const q = new URLSearchParams({ name });
    if (country) q.set("country", country);
    return this.request(`/namedays/search?${q.toString()}`);
  }
  async namedayDate(month: number, day: number): Promise<Record<string, string>> {
    return this.request(`/namedays/date?month=${month}&day=${day}`);
  }
  async namedayCountries(): Promise<string[]> {
    return this.request("/namedays/countries");
  }

  async browseHolidays(country: string, year: number): Promise<NagerHoliday[]> {
    return this.request(`/holidays/browse?country=${encodeURIComponent(country)}&year=${year}`);
  }
  async browseHolidayCountries(): Promise<Country[]> {
    return this.request("/holidays/browse/countries");
  }
  async importHolidays(country: string, year: number): Promise<{ imported: number; found: number }> {
    return this.request("/holidays/import", { method: "POST", body: { country, year } });
  }

  async carddavAccounts(): Promise<CarddavAccount[]> {
    return this.request("/carddav");
  }
  async syncCarddav(id: string): Promise<{ imported: number; found: number }> {
    return this.request(`/carddav/${id}/sync`, { method: "POST", body: "{}" });
  }
  async deleteCarddav(id: string): Promise<void> {
    await this.request(`/carddav/${id}`, { method: "DELETE" });
  }
  async carddavImportPeople(id: string): Promise<{ imported: number; found: number }> {
    return this.request(`/carddav/${id}/import-people`, { method: "POST", body: "{}" });
  }

  async forgotPassword(email: string): Promise<void> {
    await this.request("/auth/forgot", { method: "POST", body: { email } });
  }
  async resetPassword(token: string, password: string): Promise<void> {
    await this.request("/auth/reset", { method: "POST", body: { token, password } });
  }

  async filters(): Promise<SavedFilter[]> {
    return this.request("/filters");
  }
  async createFilter(name: string, filter: Record<string, unknown>): Promise<SavedFilter> {
    return this.request("/filters", { method: "POST", body: { name, filter } });
  }
  async deleteFilter(id: string): Promise<void> {
    return this.request(`/filters/${id}`, { method: "DELETE" });
  }

  async mailAccounts(): Promise<MailAccount[]> {
    return this.request("/mail/accounts");
  }
  async createMailAccount(input: {
    name?: string; email: string; imapHost: string; imapPort?: number;
    smtpHost: string; smtpPort?: number; username?: string; password: string;
    insecure?: boolean;
  }): Promise<MailAccount> {
    return this.request("/mail/accounts", { method: "POST", body: input });
  }
  async deleteMailAccount(id: string): Promise<void> {
    return this.request(`/mail/accounts/${id}`, { method: "DELETE" });
  }
  async testMailAccount(id: string): Promise<{ ok: boolean }> {
    return this.request(`/mail/accounts/${id}/test`, { method: "POST", body: "{}" });
  }
  async mailMailboxes(id: string): Promise<{ name: string; delimiter: string }[]> {
    return this.request(`/mail/${id}/mailboxes`);
  }
  async mailMessages(id: string, mailbox = "INBOX", page = 0): Promise<{ total: number; messages: MailSummary[] }> {
    return this.request(`/mail/${id}/messages?mailbox=${encodeURIComponent(mailbox)}&page=${page}`);
  }
  async mailMessage(id: string, uid: number, mailbox = "INBOX"): Promise<MailMessage> {
    return this.request(`/mail/${id}/message/${uid}?mailbox=${encodeURIComponent(mailbox)}`);
  }
  async mailFlag(id: string, uid: number, seen: boolean, mailbox = "INBOX"): Promise<void> {
    return this.request(`/mail/${id}/message/${uid}/flag?mailbox=${encodeURIComponent(mailbox)}`, { method: "POST", body: { seen } });
  }
  async mailDelete(id: string, uid: number, mailbox = "INBOX"): Promise<void> {
    return this.request(`/mail/${id}/message/${uid}?mailbox=${encodeURIComponent(mailbox)}`, { method: "DELETE" });
  }
  async mailSend(id: string, input: { to: string; cc?: string; subject: string; text: string; attachments?: string[] }): Promise<void> {
    return this.request(`/mail/${id}/send`, { method: "POST", body: input });
  }

  async githubInbox(): Promise<{ number: number; title: string; state: string; url: string; repo: string; isPR: boolean; labels: string[] }[]> {
    return this.request("/github/inbox");
  }
  async githubActivity(): Promise<{ login: string; eventsThisWeek: number }> {
    return this.request("/github/activity");
  }
  async githubImport(url: string, boardId?: string, columnId?: string): Promise<Entry> {
    return this.request("/github/import", { method: "POST", body: { url, boardId, columnId } });
  }

  async youtubeSearch(q: string): Promise<YtResult[]> {
    return this.request<YtResult[]>(`/youtube/search?q=${encodeURIComponent(q)}`);
  }

  async search(q: string): Promise<{ entries: Entry[]; files: { id: string; name: string; origName: string }[]; boards: Board[]; people: Person[] }> {
    return this.request(`/search?q=${encodeURIComponent(q)}`);
  }
  async agenda(days = 7): Promise<string> {
    const r = await fetch(`${this.root}/agenda?days=${days}`, { credentials: "include", headers: this.headers(false) });
    return r.text();
  }

  async activity(): Promise<Record<string, number>> {
    return this.request("/activity");
  }

  async pushSubscriptions(): Promise<{ id: string; label: string; endpoint: string }[]> {
    return this.request("/push/subscriptions");
  }

  async deletePushSubscription(id: string): Promise<void> {
    return this.request(`/push/subscriptions/${id}`, { method: "DELETE" });
  }

  async upload(file: File, opts: { tags?: string[]; workspaceId?: string; personId?: string } = {}): Promise<{ url: string; name: string; markdown: string }> {
    const form = new FormData();
    form.append("file", file);
    if (opts.tags?.length) form.append("tags", opts.tags.join(","));
    if (opts.workspaceId) form.append("workspaceId", opts.workspaceId);
    if (opts.personId) form.append("personId", opts.personId);
    const response = await fetch(`${this.root}/files`, {
      method: "POST",
      credentials: "include",
      headers: this.headers(false),
      body: form,
    });
    if (!response.ok) throw new Error(`Upload failed: ${response.status}`);
    return response.json();
  }

  async sessions(): Promise<SessionInfo[]> {
    return this.request<SessionInfo[]>("/sessions");
  }

  async revokeSession(id: string): Promise<void> {
    await this.request<void>(`/sessions/${id}`, { method: "DELETE" });
  }

  async changePassword(current: string, password: string): Promise<void> {
    await this.request<void>("/password", { method: "POST", body: { current, password } });
  }

  async caldavAccounts(): Promise<CaldavAccount[]> {
    return this.request<CaldavAccount[]>("/caldav");
  }

  async addCaldavAccount(input: { name?: string; url: string; username: string; password: string; color?: string }): Promise<CaldavAccount> {
    return this.request<CaldavAccount>("/caldav", { method: "POST", body: input });
  }

  async deleteCaldavAccount(id: string): Promise<void> {
    await this.request<void>(`/caldav/${id}`, { method: "DELETE" });
  }

  async syncCaldav(id: string): Promise<void> {
    await this.request<void>(`/caldav/${id}/sync`, { method: "POST" });
  }

  async entryRevisions(id: string): Promise<Revision[]> {
    return this.request<Revision[]>(`/entries/${id}/revisions`);
  }

  async restoreRevision(entryId: string, revId: string): Promise<void> {
    await this.request<void>(`/entries/${entryId}/restore/${revId}`, { method: "POST" });
  }

  async restore(file: File): Promise<{
    restored: number;
    people: number;
    links: number;
    timeline: number;
    files: number;
    filesSkipped: number;
  }> {
    const response = await fetch(`${this.root}/restore`, {
      method: "POST",
      credentials: "include",
      headers: this.headers(true),
      body: file,
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed: ${response.status}`);
    }
    return response.json();
  }

  // Dry-run a restore file: per-collection totals, new vs existing rows,
  // orphaned person references, and file rows lacking their binary.
  async restorePreview(file: File): Promise<RestorePreview> {
    const response = await fetch(`${this.root}/restore?dry=1`, {
      method: "POST",
      credentials: "include",
      headers: this.headers(true),
      body: file,
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed: ${response.status}`);
    }
    return response.json();
  }

  async importIcs(file: File): Promise<{ imported: number }> {
    const response = await fetch(`${this.root}/import`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "text/calendar", ...(this.session ? { Authorization: `Bearer ${this.session}` } : {}) },
      body: file,
    });
    if (!response.ok) {
      const text = await response.text();
      throw new ApiError(text || response.statusText, response.status);
    }
    return response.json() as Promise<{ imported: number }>;
  }

  async rotateWidgetToken(): Promise<string> {
    const out = await this.request<{ widgetToken: string }>("/settings/widget-token", { method: "POST" });
    return out.widgetToken;
  }

  async pushVapid(): Promise<string> {
    const out = await this.request<{ publicKey: string }>("/push/vapid");
    return out.publicKey;
  }

  async pushSubscribe(sub: PushSubscriptionJSON): Promise<void> {
    await this.request<void>("/push/subscribe", { method: "POST", body: sub });
  }

  async pushUnsubscribe(endpoint: string): Promise<void> {
    await this.request<void>("/push/unsubscribe", { method: "POST", body: { endpoint } });
  }

  async rotateApiToken(): Promise<string> {
    const out = await this.request<{ apiToken: string }>("/settings/api-token", { method: "POST" });
    return out.apiToken;
  }

  async rotateIntakeToken(): Promise<string> {
    const out = await this.request<{ intakeToken: string }>("/settings/intake-token", { method: "POST" });
    return out.intakeToken;
  }

  async rotateFeedToken(): Promise<string> {
    const out = await this.request<{ feedToken: string }>("/settings/feed-token", { method: "POST" });
    return out.feedToken;
  }

  /** Stores the GitHub token ("" removes it); returns whether one is set. */
  async setGitHubToken(token: string): Promise<boolean> {
    const out = token
      ? await this.request<{ githubConnected: boolean }>("/settings/github-token", { method: "PUT", body: { token } })
      : await this.request<{ githubConnected: boolean }>("/settings/github-token", { method: "DELETE" });
    return out.githubConnected;
  }

  private headers(withJson: boolean): Record<string, string> {
    const h: Record<string, string> = {};
    if (withJson) h["Content-Type"] = "application/json";
    if (this.session) h["Authorization"] = `Bearer ${this.session}`;
    return h;
  }

  private async request<T>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
    const response = await fetch(`${this.root}${path}`, {
      method: init.method ?? "GET",
      credentials: "include",
      headers: this.headers(!!init.body),
      body: init.body ? JSON.stringify(init.body) : undefined,
    });

    if (!response.ok) {
      const text = await response.text();
      throw new ApiError(text || response.statusText, response.status);
    }

    if (response.status === 204) return undefined as T;
    return response.json() as Promise<T>;
  }
}

export interface Board {
  id: string;
  name: string;
  color: string;
  description: string;
  targetDate?: string;
  shareToken?: string;
  total: number;
  done: number;
  minutes: number; // tracked time against this board
  createdAt: string;
}

export interface BoardColumn {
  id: string;
  boardId: string;
  name: string;
  position: number;
  wipLimit?: number;
}

export interface TimeEntry {
  id: string;
  entryId?: string;
  title: string;
  startAt: string;
  endAt?: string;
  note: string;
  planned?: number;
  billable?: boolean;
  rate?: number;
  projectId?: string;
  project?: string;
  tags: string[];
}

export interface Workspace {
  id: string;
  name: string;
  color: string;
  icon: string;
  position: number;
  createdAt: string;
}

export interface FeedItem {
  kind: "entry" | "file" | "card";
  action: string;
  title: string;
  entryId?: string;
  at: string;
}

export interface DashboardStats {
  tasksTotal: number;
  tasksDone: number;
  doneThisWeek: number;
  weekActivity: Record<string, number>;
  deadlines: Entry[];
  feed: FeedItem[];
  timeTodayMin: number;
  timeWeekMin: number;
  running?: TimeEntry;
}

export interface SavedFilter {
  id: string;
  name: string;
  filter: Record<string, unknown>;
  createdAt: string;
}

export interface MailAccount {
  id: string;
  name: string;
  email: string;
  imapHost: string;
  imapPort: number;
  smtpHost: string;
  smtpPort: number;
  username: string;
  /** Skip TLS verification — for self-hosted mail with self-signed certs. */
  insecure?: boolean;
  createdAt: string;
}

export interface MailSummary {
  uid: number;
  from: string;
  to?: string[];
  subject: string;
  date: string;
  seen: boolean;
  size: number;
}

export interface MailMessage {
  uid: number;
  from: string;
  to: string[];
  subject: string;
  text: string;
  html: string;
}

export interface TimeEntryInput {
  entryId?: string;
  note?: string;
  startAt?: string;
  endAt?: string;
  planned?: number;
  billable?: boolean;
  rate?: number;
  projectId?: string;
  tags?: string[];
}

export type TimeEntryPatch = Partial<{
  startAt: string | null;
  endAt: string | null;
  note: string;
  tags: string[];
  billable: boolean;
  rate: number | null;
  projectId: string | null;
  entryId: string | null;
  planned: number | null;
}>;

export interface ActivityItem {
  id: string;
  action: string;
  detail: string;
  createdAt: string;
}

export interface TimeSummary {
  todayMinutes: number;
  weekMinutes: number;
  billableAmount: number;
  perEntry: { entryId: string; title: string; minutes: number }[];
}

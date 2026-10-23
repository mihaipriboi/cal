import type { Accent, AdminConfig, CarddavAccount, Country, NagerHoliday, RestorePreview, SessionInfo, User, Webhook } from "@cal/api-client";
import { Bell, BellOff, Copy, Download, FileText, LogOut, Plus, RefreshCw, Trash2, Upload, Users } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { FeedMenu, type FeedMenuAnchor } from "../components/FeedMenu";
import { PageHeader } from "../components/PageHeader";
import { MODULES, moduleOn } from "../lib/modules";
import { disablePush, enablePush, pushEnabled } from "../lib/push";
import { reportErr, usePlanner } from "../stores/planner";

const timezones: string[] = (() => {
  const zones: string[] = typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : [];
  const local = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return local && !zones.includes(local) ? [local, ...zones] : zones;
})();

const ACCENTS: { value: Accent; label: string }[] = [
  { value: "green", label: "Forest" },
  { value: "blue", label: "Lake" },
  { value: "violet", label: "Iris" },
  { value: "amber", label: "Marigold" },
  { value: "rose", label: "Rose" },
];

export function SettingsPage() {
  const settings = usePlanner((state) => state.settings);
  const updateSettings = usePlanner((state) => state.updateSettings);
  const countries = usePlanner((state) => state.countries);
  const entries = usePlanner((state) => state.entries);
  const feeds = usePlanner((state) => state.feeds);
  const loadEntries = usePlanner((state) => state.loadEntries);
  const loadFeeds = usePlanner((state) => state.loadFeeds);
  const addFeed = usePlanner((state) => state.addFeed);
  const accounts = usePlanner((state) => state.accounts);
  const loadAccounts = usePlanner((state) => state.loadAccounts);
  const addAccount = usePlanner((state) => state.addAccount);
  const removeAccount = usePlanner((state) => state.removeAccount);
  const syncAccount = usePlanner((state) => state.syncAccount);
  const removeFeed = usePlanner((state) => state.removeFeed);
  const refreshFeed = usePlanner((state) => state.refreshFeed);
  const importIcs = usePlanner((state) => state.importIcs);
  const restore = usePlanner((state) => state.restore);
  const api = usePlanner((state) => state.api);
  const rotateToken = usePlanner((state) => state.rotateToken);
  const setGitHubToken = usePlanner((state) => state.setGitHubToken);
  const [ghToken, setGhToken] = useState("");
  const toast = usePlanner((state) => state.toast);
  const user = usePlanner((state) => state.user);
  const logout = usePlanner((state) => state.logout);
  const workspaces = usePlanner((state) => state.workspaces);
  const addWorkspace = usePlanner((state) => state.addWorkspace);
  const removeWorkspace = usePlanner((state) => state.removeWorkspace);
  const [wsName, setWsName] = useState("");

  const [feedName, setFeedName] = useState("");
  const [feedUrl, setFeedUrl] = useState("");
  const [feedKind, setFeedKind] = useState<"calendar" | "links">("calendar");
  const [feedMenu, setFeedMenu] = useState<FeedMenuAnchor>();
  const [adding, setAdding] = useState(false);
  const [pushOn, setPushOn] = useState<boolean>();
  const [davName, setDavName] = useState("");
  const [davUrl, setDavUrl] = useState("");
  const [davUser, setDavUser] = useState("");
  const [davPass, setDavPass] = useState("");
  const [addingAccount, setAddingAccount] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState<{ href: string; name: string }[]>([]);
  const [google, setGoogle] = useState<{ connected: boolean }>();
  const [cdName, setCdName] = useState("");
  const [cdUrl, setCdUrl] = useState("");
  const [cdUser, setCdUser] = useState("");
  const [cdPass, setCdPass] = useState("");
  const [cdAccounts, setCdAccounts] = useState<CarddavAccount[]>([]);
  const [namedayCountries, setNamedayCountries] = useState<string[]>([]);
  const [nagerCountries, setNagerCountries] = useState<Country[]>([]);
  const [browseCountry, setBrowseCountry] = useState("");
  const [browseYear, setBrowseYear] = useState(() => new Date().getFullYear());
  const [browsed, setBrowsed] = useState<NagerHoliday[] | null>(null);
  const [browsing, setBrowsing] = useState(false);
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [hookUrl, setHookUrl] = useState("");
  const [storage, setStorage] = useState<{ usedBytes: number; quotaBytes: number }>();
  const [pushDevices, setPushDevices] = useState<{ id: string; label: string; endpoint: string }[]>([]);
  const [pwCurrent, setPwCurrent] = useState("");
  const [pwNext, setPwNext] = useState("");
  const [adminUsers, setAdminUsers] = useState<User[]>([]);
  const [adminCfg, setAdminCfg] = useState<AdminConfig>();
  const [pwSaving, setPwSaving] = useState(false);
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const fileRef = useRef<HTMLInputElement>(null);
  const restoreRef = useRef<HTMLInputElement>(null);
  const [restorePlan, setRestorePlan] = useState<{ file: File; preview: RestorePreview } | null>(null);
  const [previewing, setPreviewing] = useState(false);

  useEffect(() => {
    void pushEnabled().then(setPushOn).catch(() => setPushOn(false));
  }, []);

  useEffect(() => {
    void loadEntries({});
    void loadFeeds();
    void loadAccounts();
    void api.sessions().then(setSessions).catch(reportErr("Could not load sessions"));
    void api.webhooks().then(setWebhooks).catch(reportErr("Could not load webhooks"));
    void api.googleStatus().then(setGoogle).catch(reportErr("Could not load Google status"));
    void api.storage().then(setStorage).catch(reportErr("Could not load storage"));
    void api.pushSubscriptions().then(setPushDevices).catch(reportErr("Could not load push devices"));
    void api.carddavAccounts().then(setCdAccounts).catch(reportErr("Could not load addressbooks"));
    void api.namedayCountries().then(setNamedayCountries).catch(() => setNamedayCountries([]));
    void api.browseHolidayCountries().then(setNagerCountries).catch(() => setNagerCountries([]));
  }, [loadEntries, loadFeeds, loadAccounts, api]);

  useEffect(() => {
    if (!user?.isAdmin) return;
    void api.adminUsers().then(setAdminUsers).catch(reportErr("Could not load users"));
    void api.adminConfig().then(setAdminCfg).catch(reportErr("Could not load access settings"));
  }, [api, user?.isAdmin]);

  async function toggleRegistration() {
    if (!adminCfg) return;
    try {
      const next = await api.adminUpdateConfig({ allowRegistration: !adminCfg.allowRegistration });
      setAdminCfg(next);
      toast(next.allowRegistration ? "Sign-ups open" : "Sign-ups closed");
    } catch (error) {
      toast(error instanceof Error ? error.message : "Failed");
    }
  }

  async function toggleUserAdmin(target: User) {
    try {
      await api.adminSetUserAdmin(target.id, !target.isAdmin);
      setAdminUsers((list) => list.map((u) => (u.id === target.id ? { ...u, isAdmin: !target.isAdmin } : u)));
    } catch (error) {
      toast(error instanceof Error ? error.message : "Failed");
    }
  }

  async function deleteUserAccount(target: User) {
    if (!window.confirm(`Delete ${target.email}? All their data is removed — this cannot be undone.`)) return;
    try {
      await api.adminDeleteUser(target.id);
      setAdminUsers((list) => list.filter((u) => u.id !== target.id));
      toast("User deleted");
    } catch (error) {
      toast(error instanceof Error ? error.message : "Failed");
    }
  }

  const stats = useMemo(() => {
    const counts = { task: 0, event: 0, note: 0, link: 0, done: 0 };
    for (const entry of entries) {
      if (entry.type === "task") {
        counts.task++;
        if (entry.completed) counts.done++;
      } else if (entry.type === "note") counts.note++;
      else if (entry.type === "event") counts.event++;
      else counts.link++;
    }
    return counts;
  }, [entries]);

  const set = (patch: Partial<typeof settings>) => void updateSettings({ ...settings, ...patch });

  async function submitPassword() {
    setPwSaving(true);
    try {
      await api.changePassword(pwCurrent, pwNext);
      setPwCurrent("");
      setPwNext("");
      void api.sessions().then(setSessions).catch(reportErr("Could not load sessions"));
      toast("Password changed");
    } catch (error) {
      toast(error instanceof Error ? error.message : "Failed");
    } finally {
      setPwSaving(false);
    }
  }

  async function revokeSession(id: string) {
    try {
      await api.revokeSession(id);
      setSessions((s) => s.filter((x) => x.id !== id));
      toast("Session signed out");
    } catch (error) {
      toast(error instanceof Error ? error.message : "Failed");
    }
  }

  async function submitAccount() {
    setAddingAccount(true);
    const ok = await addAccount({
      name: davName.trim() || undefined,
      url: davUrl.trim(),
      username: davUser.trim(),
      password: davPass,
    });
    setAddingAccount(false);
    if (ok) {
      setDavName("");
      setDavUrl("");
      setDavUser("");
      setDavPass("");
    }
  }

  async function runDiscover() {
    setDiscovering(true);
    try {
      const cols = await api.discoverCaldav({ url: davUrl.trim(), username: davUser.trim(), password: davPass });
      setDiscovered(cols);
      if (cols.length === 0) toast("No calendar collections found — check the URL");
    } catch (error) {
      toast(error instanceof Error ? error.message : "Discovery failed");
    } finally {
      setDiscovering(false);
    }
  }

  async function submitCarddav() {
    try {
      const out = await api.connectCarddav({
        name: cdName.trim() || undefined,
        url: cdUrl.trim(),
        username: cdUser.trim(),
        password: cdPass,
      });
      toast(`Imported ${out.imported} of ${out.found} birthdays`);
      setCdName(""); setCdUrl(""); setCdUser(""); setCdPass("");
      void loadEntries({});
      void api.carddavAccounts().then(setCdAccounts).catch(reportErr("Could not load addressbooks"));
    } catch (error) {
      toast(error instanceof Error ? error.message : "CardDAV connect failed");
    }
  }

  async function importPeopleFrom(id: string) {
    try {
      const out = await api.carddavImportPeople(id);
      toast(`Imported ${out.imported} of ${out.found} contacts as people`);
      void usePlanner.getState().loadPeople();
    } catch (error) {
      toast(error instanceof Error ? error.message : "People import failed");
    }
  }

  async function browseNager() {
    if (!browseCountry) return;
    setBrowsing(true);
    try {
      setBrowsed(await api.browseHolidays(browseCountry, browseYear));
    } catch (error) {
      toast(error instanceof Error ? error.message : "Browse failed");
      setBrowsed(null);
    } finally {
      setBrowsing(false);
    }
  }

  async function importNager() {
    if (!browseCountry) return;
    try {
      const out = await api.importHolidays(browseCountry, browseYear);
      toast(`Imported ${out.imported} of ${out.found} holidays`);
      void loadEntries({});
    } catch (error) {
      toast(error instanceof Error ? error.message : "Import failed");
    }
  }

  async function submitWebhook() {
    try {
      const hook = await api.addWebhook(hookUrl.trim());
      setWebhooks((w) => [...w, hook]);
      setHookUrl("");
      toast("Webhook added");
    } catch (error) {
      toast(error instanceof Error ? error.message : "Failed");
    }
  }

  async function removePushDevice(id: string) {
    try {
      await api.deletePushSubscription(id);
      setPushDevices((d) => d.filter((x) => x.id !== id));
      toast("Device removed");
    } catch (e) {
      toast(e instanceof Error ? e.message : "Could not remove device");
    }
  }

  async function removeWebhook(id: string) {
    try {
      await api.deleteWebhook(id);
      setWebhooks((w) => w.filter((x) => x.id !== id));
      toast("Webhook removed");
    } catch (e) {
      toast(e instanceof Error ? e.message : "Could not remove webhook");
    }
  }

  async function submitFeed() {
    if (!feedUrl.trim()) return;
    setAdding(true);
    const ok = await addFeed({ name: feedName.trim(), url: feedUrl.trim(), kind: feedKind });
    setAdding(false);
    if (ok) {
      setFeedName("");
      setFeedUrl("");
    }
  }

  const origin = api.remote || location.origin;
  const widgetUrl = `${origin}/widget/today?token=${settings.widgetToken}`;
  const icsUrl = `${origin}/api/feed.ics?token=${settings.feedToken ?? ""}`;
  const intakeUrl = `${origin}/api/intake?token=${settings.intakeToken ?? ""}`;
  const mcpConfig = JSON.stringify(
    { mcpServers: { cal: { url: `${origin}/api/mcp`, headers: { Authorization: `Bearer ${settings.apiToken}` } } } },
    null,
    2,
  );

  function copy(text: string, what: string) {
    void navigator.clipboard
      .writeText(text)
      .then(() => toast(`${what} copied`))
      .catch(() => toast("Copy failed — clipboard unavailable"));
  }

  return (
    <>
      <PageHeader title="Settings" />
      <div className="page-scroll settings-page">
        <section className="panel">
          <h3>Appearance</h3>
          <div className="settings-grid">
            <label className="field">
              <span>Theme</span>
              <select
                className="select"
                value={settings.theme}
                onChange={(e) => set({ theme: e.target.value as typeof settings.theme })}
              >
                <option value="system">System</option>
                <option value="dark">Dark</option>
                <option value="light">Light</option>
              </select>
            </label>
            <label className="field">
              <span>Default view</span>
              <select
                className="select"
                value={settings.defaultView ?? "month"}
                onChange={(e) => set({ defaultView: e.target.value as typeof settings.defaultView })}
              >
                <option value="month">Month</option>
                <option value="week">Week</option>
                <option value="day">Day</option>
              </select>
            </label>
            <div className="field">
              <span>Accent</span>
              <div className="dots">
                {ACCENTS.map(({ value, label }) => (
                  <button
                    key={value}
                    type="button"
                    className={`dot accent-${value} ${settings.accent === value ? "active" : ""}`}
                    onClick={() => set({ accent: value })}
                    aria-label={`Accent ${label}`}
                    title={label}
                  />
                ))}
              </div>
            </div>
          </div>
        </section>

        <section className="panel">
          <h3>General</h3>
          <div className="settings-grid">
            <label className="field">
              <span>Week starts on</span>
              <select
                className="select"
                value={settings.weekStart}
                onChange={(e) => set({ weekStart: e.target.value as typeof settings.weekStart })}
              >
                <option value="monday">Monday</option>
                <option value="sunday">Sunday</option>
              </select>
            </label>
            <label className="field">
              <span>Timezone</span>
              <select
                className="select"
                value={settings.timezone || "UTC"}
                onChange={(e) => set({ timezone: e.target.value })}
              >
                {timezones.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>Morning digest push</span>
              <input
                type="time"
                className="input"
                value={settings.digestTime ?? ""}
                onChange={(e) => set({ digestTime: e.target.value })}
                aria-label="Daily digest time"
              />
            </label>
            <label className="field">
              <span>Weather city</span>
              <input
                className="input"
                placeholder="e.g. Prague — blank hides the strip"
                value={settings.city}
                onChange={(e) => set({ city: e.target.value })}
              />
            </label>
            <label className="field">
              <span>Default hourly rate</span>
              <input
                type="number"
                min="0"
                step="0.01"
                className="input"
                placeholder="$/h for billable timers"
                value={settings.defaultRate ?? ""}
                onChange={(e) => set({ defaultRate: e.target.value === "" ? undefined : Number(e.target.value) })}
              />
            </label>
            <label className="field">
              <span>Quick-added tasks with no day</span>
              <select
                className="select"
                value={settings.quickAddToday ? "today" : "none"}
                onChange={(e) => set({ quickAddToday: e.target.value === "today" })}
              >
                <option value="none">Go to No date</option>
                <option value="today">Go to Today</option>
              </select>
            </label>
            <label className="field">
              <span>No date group in Tasks</span>
              <select
                className="select"
                value={settings.undatedFirst ? "first" : "last"}
                onChange={(e) => set({ undatedFirst: e.target.value === "first" })}
              >
                <option value="last">Shown last</option>
                <option value="first">Shown first</option>
              </select>
            </label>
          </div>
        </section>

        <section className="panel">
          <h3>Integrations</h3>
          <div className="settings-grid">
            <div className="field">
              <span>GitHub token{settings.githubConnected ? " — saved" : ""}</span>
              <div className="token-row">
                <input
                  type="password"
                  className="input"
                  aria-label="GitHub token"
                  autoComplete="off"
                  placeholder={settings.githubConnected ? "Paste a new token to replace it" : "Personal access token — issues & PRs"}
                  value={ghToken}
                  onChange={(e) => setGhToken(e.target.value)}
                />
                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={!ghToken.trim()}
                  onClick={() => void setGitHubToken(ghToken.trim()).then((ok) => ok && setGhToken(""))}
                >
                  Save
                </button>
                {settings.githubConnected && (
                  <button type="button" className="btn btn-secondary" onClick={() => void setGitHubToken("")}>
                    Remove
                  </button>
                )}
              </div>
            </div>
            <label className="switch-row">
              Close the GitHub issue when its card is completed
              <span className="switch">
                <input
                  type="checkbox"
                  checked={settings.githubCloseIssues ?? false}
                  onChange={(e) => set({ githubCloseIssues: e.target.checked })}
                />
                <i />
              </span>
            </label>
            <label className="field">
              <span>Invidious instance</span>
              <input
                type="url"
                className="input"
                placeholder="https://invidious.example — enables YouTube search on Links"
                value={settings.invidiousUrl ?? ""}
                onChange={(e) => set({ invidiousUrl: e.target.value })}
              />
            </label>
          </div>
        </section>

        <section className="panel">
          <h3>Workspaces</h3>
          <p className="panel-note">Separate scopes for entries, files and time — Work, Personal, anything. Deleting a space returns its contents to Personal.</p>
          <ul className="check-list" style={{ marginBottom: 10 }}>
            {workspaces.map((w) => (
              <li key={w.id}>
                <span className="swatch" style={{ "--swatch": `var(--c-${w.color})` } as React.CSSProperties} />
                <span className="row-title">{w.name}</span>
                <button
                  type="button"
                  className="icon-btn danger"
                  aria-label={`Delete ${w.name}`}
                  onClick={() => void removeWorkspace(w.id)}
                >
                  ×
                </button>
              </li>
            ))}
            {workspaces.length === 0 && <li className="panel-empty">No spaces yet — everything lives in Personal.</li>}
          </ul>
          <div className="field-row">
            <input
              className="input"
              placeholder="New space name"
              value={wsName}
              onChange={(e) => setWsName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && wsName.trim() && void addWorkspace({ name: wsName.trim() }).then(() => setWsName(""))}
            />
            <button
              type="button"
              className="btn btn-secondary"
              disabled={!wsName.trim()}
              onClick={() => void addWorkspace({ name: wsName.trim() }).then((w) => w && setWsName(""))}
            >
              Create
            </button>
          </div>
        </section>

        <section className="panel">
          <h3>Modules</h3>
          <p className="panel-note">Turn off what you don't use — hidden from navigation, routes and search.</p>
          <div className="module-grid">
            {MODULES.map((m) => {
              const on = moduleOn(settings, m.key);
              return (
                <button
                  key={m.key}
                  type="button"
                  className={`module-toggle ${on ? "on" : ""}`}
                  aria-pressed={on}
                  title={m.hint}
                  onClick={() => set({ modules: { ...settings.modules, [m.key]: !on } })}
                >
                  <span className="module-dot" />
                  {m.label}
                  <i>{on ? "on" : "off"}</i>
                </button>
              );
            })}
          </div>
        </section>

        <section className="panel">
          <h3>Feeds</h3>
          <p className="panel-note">
            Subscribe to iCalendar (.ics) or RSS/Atom URLs — Google Calendar's secret address,
            iCloud public calendars, blog feeds. "To links" feeds (e.g. a YouTube channel's{" "}
            <code>feeds/videos.xml</code>) save new items as link cards with thumbnails instead.
            Click a feed's colour to rename or recolour it.
          </p>
          {feeds.map((feed) => (
            <div key={feed.id} className="feed-row">
              <button
                type="button"
                className="swatch-btn"
                style={{ "--swatch": `var(--c-${feed.color})` } as React.CSSProperties}
                aria-label={`Rename or recolour ${feed.name}`}
                onClick={(e) => {
                  const r = e.currentTarget.getBoundingClientRect();
                  setFeedMenu({ feed, x: r.left, y: r.bottom + 6 });
                }}
              />
              <div className="feed-meta">
                <span className="feed-name">{feed.name}{feed.kind === "links" && <span className="feed-kind">→ links</span>}</span>
                <span className="feed-url">{feed.url}</span>
              </div>
              {feed.fetchedAt && (
                <span className="feed-age">synced {new Date(feed.fetchedAt).toLocaleTimeString()}</span>
              )}
              <button type="button" className="icon-btn" aria-label="Refresh feed" onClick={() => void refreshFeed(feed.id)}>
                <RefreshCw size={14} />
              </button>
              <button type="button" className="icon-btn" aria-label="Delete feed" onClick={() => void removeFeed(feed.id)}>
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          {feedMenu && <FeedMenu {...feedMenu} onClose={() => setFeedMenu(undefined)} />}
          <div className="feed-add">
            <input className="input" placeholder="Name (e.g. Work)" value={feedName} onChange={(e) => setFeedName(e.target.value)} />
            <input
              className="input"
              placeholder="https://calendar.google.com/…/basic.ics"
              value={feedUrl}
              onChange={(e) => setFeedUrl(e.target.value)}
            />
            <select className="input feed-kind-select" value={feedKind} onChange={(e) => setFeedKind(e.target.value as "calendar" | "links")} aria-label="Feed kind">
              <option value="calendar">→ calendar</option>
              <option value="links">→ links</option>
            </select>
            <button type="button" className="btn btn-primary" disabled={adding || !feedUrl.trim()} onClick={() => void submitFeed()}>
              <Plus size={14} /> Add
            </button>
          </div>
          <p className="panel-note" style={{ marginTop: 10 }}>
            Or import a file once:
            <input
              ref={fileRef}
              type="file"
              accept=".ics,text/calendar"
              style={{ display: "none" }}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) void importIcs(file);
                e.target.value = "";
              }}
            />
            <button type="button" className="btn btn-secondary" style={{ marginLeft: 8 }} onClick={() => fileRef.current?.click()}>
              <Upload size={14} /> Import .ics
            </button>
          </p>
        </section>

        <section className="panel">
          <h3>Connected calendars</h3>
          <p className="panel-note">
            Two-way sync over CalDAV — Nextcloud, Radicale, Baikal, Fastmail (app password). Events on a
            connected calendar stay editable in both directions. Paste the collection URL
            (e.g. <code>https://cloud.example/remote.php/dav/calendars/you/personal/</code>).
          </p>
          {accounts.map((a) => (
            <div key={a.id} className="feed-row">
              <span className="swatch" style={{ "--swatch": `var(--c-${a.color})` } as React.CSSProperties} />
              <div className="feed-meta">
                <span className="feed-name">{a.name}</span>
                <span className="feed-url">{a.url}</span>
              </div>
              {a.lastSynced && <span className="feed-age">synced {new Date(a.lastSynced).toLocaleTimeString()}</span>}
              <button type="button" className="icon-btn" aria-label="Sync now" onClick={() => void syncAccount(a.id)}>
                <RefreshCw size={14} />
              </button>
              <button type="button" className="icon-btn" aria-label="Disconnect" onClick={() => void removeAccount(a.id)}>
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          <div className="feed-add" style={{ gridTemplateColumns: "1fr 1fr" }}>
            <input className="input" placeholder="Name" value={davName} onChange={(e) => setDavName(e.target.value)} />
            <input className="input" placeholder="Calendar URL" value={davUrl} onChange={(e) => setDavUrl(e.target.value)} />
            <input className="input" placeholder="Username" value={davUser} onChange={(e) => setDavUser(e.target.value)} />
            <input
              className="input"
              type="password"
              placeholder="Password / app password"
              value={davPass}
              onChange={(e) => setDavPass(e.target.value)}
            />
          </div>
          {discovered.length > 0 && (
            <div className="discover-list">
              {discovered.map((c) => (
                <button
                  key={c.href}
                  type="button"
                  className="feed-row as-btn"
                  onClick={() => {
                    setDavUrl(c.href);
                    if (!davName) setDavName(c.name);
                  }}
                >
                  <div className="feed-meta">
                    <span className="feed-name">{c.name}</span>
                    <span className="feed-url">{c.href}</span>
                  </div>
                </button>
              ))}
            </div>
          )}
          <div style={{ marginTop: 8, display: "flex", justifyContent: "flex-end", gap: 8 }}>
            <button
              type="button"
              className="btn btn-secondary"
              disabled={discovering || !davUrl.trim() || !davUser.trim()}
              onClick={() => void runDiscover()}
            >
              {discovering ? "Discovering…" : "Discover calendars"}
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={addingAccount || !davUrl.trim()}
              onClick={() => void submitAccount()}
            >
              <Plus size={14} /> {addingAccount ? "Connecting…" : "Connect"}
            </button>
          </div>
          <p className="panel-note" style={{ marginTop: 8 }}>
            Proton Calendar does not expose CalDAV outside Bridge — import via .ics instead. For Google,
            connect below (OAuth) or use its .ics secret address above.
          </p>
        </section>

        <section className="panel">
          <h3>Google Calendar</h3>
          {google === undefined ? null : google.connected ? (
            <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
              <span className="feed-name">Connected — events sync every 15 min as the "Google" feed.</span>
              <span className="spacer" style={{ flex: 1 }} />
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() =>
                  void api
                    .googleSync()
                    .then(() => toast("Synced"))
                    .catch((e) => toast(e instanceof Error ? e.message : "Sync failed"))
                }
              >
                Sync now
              </button>
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() =>
                  void api
                    .googleDisconnect()
                    .then(() => { setGoogle({ connected: false }); void loadFeeds(); })
                    .catch((e) => toast(e instanceof Error ? e.message : "Disconnect failed"))
                }
              >
                Disconnect
              </button>
            </div>
          ) : (
            <div>
              <p className="panel-note">
                Requires <code>GOOGLE_CLIENT_ID</code>/<code>GOOGLE_CLIENT_SECRET</code> on the API
                (Google Cloud → OAuth consent → Calendar read scope). Events sync read-only into a
                "Google" feed — toggleable like any other feed.
              </p>
              <button
                type="button"
                className="btn btn-primary"
                onClick={() =>
                  void api
                    .googleConnect()
                    .then((r) => window.open(r.url, "_blank", "noopener"))
                    .catch((e) => toast(e instanceof Error ? e.message : "Google not configured"))
                }
              >
                Connect Google
              </button>
            </div>
          )}
        </section>

        <section className="panel">
          <h3>Birthdays (CardDAV)</h3>
          <p className="panel-note">
            Connect an addressbook (Nextcloud contacts, Radicale, Baikal) — contacts with a birthday
            become yearly all-day events, or full people profiles.
          </p>
          {cdAccounts.map((a) => (
            <div key={a.id} className="feed-row">
              <div className="feed-meta">
                <span className="feed-name">{a.name}</span>
                <span className="feed-url">{a.url}</span>
              </div>
              {a.lastSynced && <span className="feed-age">synced {new Date(a.lastSynced).toLocaleDateString()}</span>}
              <button
                type="button"
                className="btn btn-secondary btn-xs"
                title="Create people profiles from contacts"
                onClick={() => void importPeopleFrom(a.id)}
              >
                <Users size={12} /> As people
              </button>
              <button
                type="button"
                className="icon-btn"
                aria-label="Sync birthdays"
                title="Re-import birthday events"
                onClick={() =>
                  void api
                    .syncCarddav(a.id)
                    .then((out) => { toast(`Imported ${out.imported} of ${out.found} birthdays`); void loadEntries({}); })
                    .catch((e) => toast(e instanceof Error ? e.message : "Sync failed"))
                }
              >
                <RefreshCw size={14} />
              </button>
              <button
                type="button"
                className="icon-btn"
                aria-label="Remove addressbook"
                onClick={() =>
                  void api
                    .deleteCarddav(a.id)
                    .then(() => setCdAccounts((s) => s.filter((x) => x.id !== a.id)))
                    .catch((e) => toast(e instanceof Error ? e.message : "Remove failed"))
                }
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          <div className="feed-add" style={{ gridTemplateColumns: "1fr 1fr" }}>
            <input className="input" placeholder="Name" value={cdName} onChange={(e) => setCdName(e.target.value)} />
            <input className="input" placeholder="Addressbook URL" value={cdUrl} onChange={(e) => setCdUrl(e.target.value)} />
            <input className="input" placeholder="Username" value={cdUser} onChange={(e) => setCdUser(e.target.value)} />
            <input
              className="input"
              type="password"
              placeholder="Password / app password"
              value={cdPass}
              onChange={(e) => setCdPass(e.target.value)}
            />
          </div>
          <div style={{ marginTop: 8, display: "flex", justifyContent: "flex-end" }}>
            <button type="button" className="btn btn-primary" disabled={!cdUrl.trim()} onClick={() => void submitCarddav()}>
              <Plus size={14} /> Import birthdays
            </button>
          </div>
        </section>

        <section className="panel">
          <h3>Webhooks</h3>
          <p className="panel-note">
            Cal POSTs <code>{"{event, entry}"}</code> to each URL on create/update/delete, signed with
            HMAC-SHA256 in <code>X-Cal-Signature</code> (verify with the secret shown once).
          </p>
          {webhooks.map((w) => (
            <div key={w.id} className="feed-row">
              <div className="feed-meta">
                <span className="feed-name">{w.url}</span>
                <span className="feed-url">secret: {w.secret}</span>
              </div>
              <button type="button" className="icon-btn" aria-label="Remove webhook" onClick={() => void removeWebhook(w.id)}>
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          <div className="feed-add">
            <input className="input" placeholder="https://example.com/hook" value={hookUrl} onChange={(e) => setHookUrl(e.target.value)} />
            <button type="button" className="btn btn-primary" disabled={!hookUrl.trim()} onClick={() => void submitWebhook()}>
              <Plus size={14} /> Add
            </button>
          </div>
          <p className="panel-note" style={{ marginTop: 8 }}>
            Email-to-task: POST <code>{"{subject, text}"}</code> to this URL — lands as a task tagged{" "}
            <code>inbox</code>. Its token can only create tasks.
          </p>
          <div className="token-row">
            <code className="token">{intakeUrl}</code>
            <button type="button" className="icon-btn" aria-label="Copy intake URL" onClick={() => copy(intakeUrl, "URL")}>
              <Copy size={14} />
            </button>
            <button type="button" className="icon-btn" aria-label="Rotate intake token" onClick={() => void rotateToken("intake")}>
              <RefreshCw size={14} />
            </button>
          </div>
        </section>

        <section className="panel">
          <h3>Holidays</h3>
          <div className="settings-grid">
            <label className="switch-row">
              Show holidays
              <span className="switch">
                <input
                  type="checkbox"
                  checked={settings.showHolidays}
                  onChange={(e) => set({ showHolidays: e.target.checked })}
                />
                <i />
              </span>
            </label>
            <label className="field">
              <span>Region</span>
              <select className="select" value={settings.country} onChange={(e) => set({ country: e.target.value })}>
                {countries.length === 0 && <option value={settings.country}>{settings.country}</option>}
                {countries.map((country) => (
                  <option key={country.code} value={country.code}>
                    {country.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="field">
              <span>Nameday country</span>
              <select
                className="select"
                value={settings.namedayCountry ?? ""}
                onChange={(e) => set({ namedayCountry: e.target.value })}
                title="Default country for nameday lookups in the person editor"
              >
                <option value="">All countries</option>
                {namedayCountries.map((c) => (
                  <option key={c} value={c}>
                    {c.toUpperCase()}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {nagerCountries.length > 0 && (
            <div className="nager-browse">
              <p className="panel-note">
                Extended coverage — browse public holidays for ~150 countries (date.nager.at) and
                import them as all-day entries.
              </p>
              <div className="feed-add" style={{ gridTemplateColumns: "1fr 110px auto auto" }}>
                <select
                  className="select"
                  value={browseCountry}
                  aria-label="Holiday country"
                  onChange={(e) => { setBrowseCountry(e.target.value); setBrowsed(null); }}
                >
                  <option value="">Country…</option>
                  {nagerCountries.map((c) => (
                    <option key={c.code} value={c.code}>
                      {c.name}
                    </option>
                  ))}
                </select>
                <input
                  className="input"
                  type="number"
                  min={1900}
                  max={2100}
                  value={browseYear}
                  aria-label="Year"
                  onChange={(e) => { setBrowseYear(Number(e.target.value) || browseYear); setBrowsed(null); }}
                />
                <button type="button" className="btn btn-secondary" disabled={!browseCountry || browsing} onClick={() => void browseNager()}>
                  {browsing ? "Loading…" : "Browse"}
                </button>
                <button type="button" className="btn btn-primary" disabled={!browseCountry} onClick={() => void importNager()}>
                  <Download size={14} /> Import
                </button>
              </div>
              {browsed && (
                <ul className="nager-list">
                  {browsed.map((h) => (
                    <li key={`${h.date}-${h.name}`}>
                      <span className="nager-date">{h.date.slice(5).replace("-", "/")}</span>
                      {h.name}
                      {!h.nationalHoliday && <span className="meta-chip">regional</span>}
                    </li>
                  ))}
                  {browsed.length === 0 && <li className="panel-empty">No holidays returned.</li>}
                </ul>
              )}
            </div>
          )}
        </section>

        <section className="panel">
          <h3>Notifications</h3>
          <p className="panel-note">
            Entries with a reminder fire a notification at start − lead time. Push works even when the
            tab is closed; in-app notifications cover the open app.
          </p>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => {
              if (pushOn) {
                void disablePush(usePlanner.getState().api)
                  .then(() => setPushOn(false))
                  .catch(() => toast("Could not disable push"));
              } else {
                void enablePush(usePlanner.getState().api)
                  .then((ok) => {
                    setPushOn(ok);
                    toast(ok ? "Push notifications enabled" : "Notifications not available here");
                  })
                  .catch(() => toast("Could not enable notifications"));
              }
            }}
          >
            {pushOn ? <BellOff size={14} /> : <Bell size={14} />}
            {pushOn ? "Disable push notifications" : "Enable push notifications"}
          </button>
          {pushDevices.length > 0 && (
            <div style={{ marginTop: 12 }}>
              <span className="panel-note" style={{ display: "block", marginBottom: 6 }}>
                Devices receiving push:
              </span>
              {pushDevices.map((d) => (
                <div key={d.id} className="feed-row">
                  <div className="feed-meta">
                    <span className="feed-name">{d.label || "Device"}</span>
                    <span className="feed-url">{d.endpoint}</span>
                  </div>
                  <button
                    type="button"
                    className="icon-btn"
                    aria-label="Revoke device"
                    onClick={() => void removePushDevice(d.id)}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>

        <section className="panel">
          <h3>MCP / API access</h3>
          <p className="panel-note">
            Cal exposes a Model Context Protocol endpoint so assistants (Claude, Devin, others) can read and
            manage your planner. Token is full-access — keep it private.
          </p>
          <div className="token-row">
            <code className="token">{settings.apiToken}</code>
            <button type="button" className="icon-btn" aria-label="Copy token" onClick={() => copy(settings.apiToken, "Token")}>
              <Copy size={14} />
            </button>
            <button
              type="button"
              className="icon-btn"
              aria-label="Rotate token"
              onClick={() => void rotateToken("api")}
            >
              <RefreshCw size={14} />
            </button>
          </div>
          <pre className="code-block" tabIndex={0}>{mcpConfig}</pre>
          <button type="button" className="btn btn-secondary" onClick={() => copy(mcpConfig, "Config")}>
            <Copy size={14} /> Copy client config
          </button>
        </section>

        <section className="panel">
          <h3>Today widget</h3>
          <p className="panel-note">
            A minimal read-only agenda for embedding in dashboards (Homepage, Heimdall, an iframe, a TV).
          </p>
          <div className="token-row">
            <code className="token">{widgetUrl}</code>
            <button type="button" className="icon-btn" aria-label="Copy widget URL" onClick={() => copy(widgetUrl, "URL")}>
              <Copy size={14} />
            </button>
            <button
              type="button"
              className="icon-btn"
              aria-label="Rotate widget token"
              onClick={() => void rotateToken("widget")}
            >
              <RefreshCw size={14} />
            </button>
          </div>
          <pre className="code-block" tabIndex={0}>{`<iframe src="${widgetUrl}" style="border:0;width:100%;height:320px"></iframe>`}</pre>
          <p className="panel-note" style={{ marginTop: 10 }}>
            Or subscribe to Cal itself in any calendar app — this URL serves a live .ics feed of your tasks and
            events (its own token, separate from the widget's):
          </p>
          <div className="token-row">
            <code className="token">{icsUrl}</code>
            <button type="button" className="icon-btn" aria-label="Copy feed URL" onClick={() => copy(icsUrl, "URL")}>
              <Copy size={14} />
            </button>
            <button type="button" className="icon-btn" aria-label="Rotate feed token" onClick={() => void rotateToken("feed")}>
              <RefreshCw size={14} />
            </button>
          </div>
        </section>

        <section className="panel">
          <h3>Your data</h3>
          <p className="panel-note">
            {stats.task} tasks ({stats.done} done), {stats.event} events, {stats.note} notes, {stats.link}{" "}
            links — stored in your own database. Take them with you any time.
          </p>
          {storage && (
            <p className="panel-note">
              Attachments: {(storage.usedBytes / 1048576).toFixed(1)} MB of{" "}
              {(storage.quotaBytes / 1048576).toFixed(0)} MB used.
            </p>
          )}
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <a className="btn btn-secondary" href="/api/export" download>
              <Download size={14} /> Export (JSON)
            </a>
            <a className="btn btn-secondary" href="/api/export?format=zip" download>
              <Download size={14} /> Export + files (zip)
            </a>
            <a className="btn btn-secondary" href="/api/agenda?days=7" target="_blank" rel="noreferrer">
              <FileText size={14} /> Week agenda (markdown)
            </a>
            <input
              ref={restoreRef}
              type="file"
              accept=".json,.zip,application/json,application/zip"
              style={{ display: "none" }}
              onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                if (!file) return;
                setPreviewing(true);
                void api
                  .restorePreview(file)
                  .then((preview) => setRestorePlan({ file, preview }))
                  .catch(reportErr("Not a Cal export file"))
                  .finally(() => setPreviewing(false));
              }}
            />
            <button type="button" className="btn btn-secondary" onClick={() => restoreRef.current?.click()}>
              <Upload size={14} /> {previewing ? "Reading…" : "Restore from backup"}
            </button>
          </div>
          {restorePlan && (
            <div className="panel" style={{ marginTop: 10 }}>
              <p className="panel-note" style={{ marginBottom: 8 }}>
                <strong>{restorePlan.file.name}</strong> — restore merges into your current data:
              </p>
              <ul className="panel-note" style={{ margin: "0 0 10px 18px", padding: 0 }}>
                <li>
                  {restorePlan.preview.entries.new} entries
                  {restorePlan.preview.entries.existing > 0 && ` (${restorePlan.preview.entries.existing} already present)`}
                </li>
                {restorePlan.preview.people.total > 0 && (
                  <li>
                    {restorePlan.preview.people.new} people
                    {restorePlan.preview.people.existing > 0 && ` (${restorePlan.preview.people.existing} already present)`}
                  </li>
                )}
                {restorePlan.preview.links.total > 0 && (
                  <li>
                    {restorePlan.preview.links.new} relations
                    {restorePlan.preview.links.orphaned > 0 && ` (${restorePlan.preview.links.orphaned} skipped — person missing)`}
                  </li>
                )}
                {restorePlan.preview.timeline.total > 0 && (
                  <li>
                    {restorePlan.preview.timeline.new} timeline items
                    {restorePlan.preview.timeline.orphaned > 0 && ` (${restorePlan.preview.timeline.orphaned} skipped — person missing)`}
                  </li>
                )}
                {restorePlan.preview.files.total > 0 && (
                  <li>
                    {restorePlan.preview.files.new} files
                    {restorePlan.preview.files.noBinary > 0 && ` (${restorePlan.preview.files.noBinary} skipped — no binary in archive)`}
                  </li>
                )}
              </ul>
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={() => {
                    const file = restorePlan.file;
                    setRestorePlan(null);
                    void restore(file);
                  }}
                >
                  Restore
                </button>
                <button type="button" className="btn btn-secondary" onClick={() => setRestorePlan(null)}>
                  Cancel
                </button>
              </div>
            </div>
          )}
          <p className="panel-note" style={{ marginTop: 10 }}>
            JSON carries everything except upload binaries; the zip adds them. A restore merges — data already
            present is kept, missing pieces come back (accepts .json or .zip). The server also writes a nightly
            backup to <code>DATA_DIR/backups/</code> (14 days kept).
          </p>
        </section>

        <section className="panel">
          <h3>Account</h3>
          <p className="panel-note">{user?.email}</p>
          <button type="button" className="btn btn-secondary" onClick={() => void logout()}>
            <LogOut size={14} /> Sign out
          </button>
        </section>

        <section className="panel">
          <h3>Security</h3>
          <div className="feed-add" style={{ gridTemplateColumns: "1fr 1fr auto" }}>
            <input
              className="input"
              type="password"
              placeholder="Current password"
              value={pwCurrent}
              onChange={(e) => setPwCurrent(e.target.value)}
            />
            <input
              className="input"
              type="password"
              placeholder="New password (8+ chars)"
              maxLength={72}
              value={pwNext}
              onChange={(e) => setPwNext(e.target.value)}
            />
            <button
              type="button"
              className="btn btn-primary"
              disabled={pwSaving || pwCurrent === "" || pwNext.length < 8}
              onClick={() => void submitPassword()}
            >
              {pwSaving ? "Saving…" : "Change"}
            </button>
          </div>
          <p className="panel-note" style={{ marginTop: 10 }}>
            Changing the password signs out every other session.
          </p>
          <h4 style={{ margin: "14px 0 8px" }}>Active sessions</h4>
          {sessions.map((s) => (
            <div key={s.id} className="feed-row">
              <div className="feed-meta">
                <span className="feed-name">
                  {s.current ? "This device" : s.userAgent || "Unknown device"}
                </span>
                <span className="feed-url">
                  {s.lastSeen ? `active ${new Date(s.lastSeen).toLocaleString()}` : `since ${new Date(s.createdAt).toLocaleDateString()}`}
                  {!s.current && s.userAgent ? ` — ${s.userAgent.slice(0, 60)}` : ""}
                </span>
              </div>
              {!s.current && (
                <button type="button" className="icon-btn" aria-label="Revoke session" onClick={() => void revokeSession(s.id)}>
                  <Trash2 size={14} />
                </button>
              )}
            </div>
          ))}
        </section>

        {user?.isAdmin && (
          <section className="panel">
            <h3>Users &amp; access</h3>
            <p className="panel-note">
              Instance administration. The first registered account is the administrator;
              admins manage who can sign up and who keeps an account. Sign-ups are closed
              by default once that first account exists; open them to invite someone.
            </p>
            {adminCfg && (
              <label className="switch-row" style={{ marginBottom: 12 }}>
                Allow new registrations
                <span className="switch">
                  <input
                    type="checkbox"
                    checked={adminCfg.allowRegistration}
                    onChange={() => void toggleRegistration()}
                  />
                  <i />
                </span>
              </label>
            )}
            {adminUsers.map((u) => (
              <div key={u.id} className="feed-row">
                <div className="feed-meta">
                  <span className="feed-name">
                    {u.email}
                    {u.isAdmin && <span className="feed-kind">admin</span>}
                    {u.id === user.id && <span className="feed-kind">you</span>}
                  </span>
                  <span className="feed-url">
                    joined {u.createdAt ? new Date(u.createdAt).toLocaleDateString() : "—"}
                  </span>
                </div>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => void toggleUserAdmin(u)}
                >
                  {u.isAdmin ? "Remove admin" : "Make admin"}
                </button>
                {u.id !== user.id && (
                  <button
                    type="button"
                    className="icon-btn"
                    aria-label={`Delete ${u.email}`}
                    onClick={() => void deleteUserAccount(u)}
                  >
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
            ))}
          </section>
        )}
      </div>
    </>
  );
}

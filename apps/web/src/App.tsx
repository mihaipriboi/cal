import { CalendarDays } from "lucide-react";
import { moduleForPath, moduleOn } from "./lib/modules";
import { useEffect, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { AuthPanel } from "./components/AuthPanel";
import { BottomNav } from "./components/BottomNav";
import { CommandPalette } from "./components/CommandPalette";
import { ContextMenu } from "./components/ContextMenu";
import { EntryEditor } from "./components/EntryEditor";
import { EventCard } from "./components/EventCard";
import { LocalShell } from "./components/LocalShell";
import { Sidebar } from "./components/Sidebar";
import { Toasts } from "./components/Toasts";
import { ZoneBanner } from "./components/ZoneBanner";
import { iso, zonedNow } from "./lib/date";
import { CalendarPage } from "./pages/CalendarPage";
import { BoardsPage } from "./pages/BoardsPage";
import { SharedBoardPage } from "./pages/SharedBoardPage";
import { SharedPeoplePage } from "./pages/SharedPeoplePage";
import { TagsPage } from "./pages/TagsPage";
import { TimePage } from "./pages/TimePage";
import { TrashPage } from "./pages/TrashPage";
import { FilesPage } from "./pages/FilesPage";
import { GitHubPage } from "./pages/GitHubPage";
import { LinksPage } from "./pages/LinksPage";
import { MailPage } from "./pages/MailPage";
import { NotesPage } from "./pages/NotesPage";
import { PeoplePage } from "./pages/PeoplePage";
import { PersonProfilePage } from "./pages/PersonProfilePage";
import { FamilyTreePage } from "./pages/FamilyTreePage";
import { SettingsPage } from "./pages/SettingsPage";
import { ShoppingPage } from "./pages/ShoppingPage";
import { TasksPage } from "./pages/TasksPage";
import { TodayPage } from "./pages/TodayPage";
import { WidgetPage } from "./pages/WidgetPage";
import { usePlanner } from "./stores/planner";
import { useUi } from "./stores/ui";

function Shell() {
  const { user, mode } = usePlanner();
  const { paletteOpen, editor, contextMenu, eventCard, selectedDate } = useUi();
  const { setView, shift, goToday, openPalette, closePalette, openCreate, setContextMenu, closeSidebar } = useUi();
  const location = useLocation();
  const navigate = useNavigate();
  const onCalendar = location.pathname === "/";

  // Global keyboard shortcuts. View-specific ones apply on the calendar only.
  useEffect(() => {
    if (!user || mode === "local") return;
    const onKey = (event: KeyboardEvent) => {
      const tag = (event.target as HTMLElement).tagName;
      const typing = tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        paletteOpen ? closePalette() : openPalette();
        return;
      }
      if (typing || paletteOpen || editor.mode !== "closed" || contextMenu || eventCard) return;
      switch (event.key) {
        case "1":
        case "2":
        case "3":
          if (onCalendar) setView(event.key === "1" ? "month" : event.key === "2" ? "week" : "day");
          break;
        case "t":
          if (onCalendar) goToday();
          else navigate("/today");
          break;
        case "c":
        case "n":
          openCreate(selectedDate);
          break;
        case "/":
          event.preventDefault();
          openPalette();
          break;
        case "ArrowLeft":
        case "ArrowRight":
          if (onCalendar) shift(event.key === "ArrowLeft" ? -1 : 1);
          break;
        case "Escape":
          setContextMenu(undefined);
          closeSidebar();
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [user, mode, paletteOpen, editor.mode, contextMenu, eventCard, selectedDate, onCalendar, navigate, setView, shift, goToday, openPalette, closePalette, openCreate, setContextMenu, closeSidebar]);

  // Notification reminder loop — checks every 30s for entries whose
  // start time minus `remind` minutes has arrived. Fires once per entry.
  useEffect(() => {
    if (!user || mode === "local") return;
    if (typeof Notification === "undefined") return;
    if (Notification.permission === "default") void Notification.requestPermission();
    const fired = new Set<string>();
    const check = () => {
      if (Notification.permission !== "granted") return;
      // Entry times are wall clock in the Settings time zone.
      const now = zonedNow();
      const today = iso(now);
      for (const entry of usePlanner.getState().entries) {
        if (entry.remind === undefined || entry.remind === null || !entry.startTime || entry.date !== today) continue;
        if (fired.has(entry.id)) continue;
        const [h, m] = entry.startTime.split(":").map(Number);
        const at = new Date(now);
        at.setHours(h, m - entry.remind, 0, 0);
        if (now >= at) {
          fired.add(entry.id);
          new Notification(entry.title, {
            body: entry.remind === 0 ? "Starting now" : `Starts at ${entry.startTime}`,
            tag: entry.id,
            icon: "/pwa-192.png",
          });
        }
      }
    };
    check();
    const timer = window.setInterval(check, 30_000);
    return () => window.clearInterval(timer);
  }, [user, mode]);

  // Public shared pages render without auth.
  if (window.location.pathname.startsWith("/board/") || window.location.pathname.startsWith("/people/shared/")) {
    return (
      <Routes>
        <Route path="/board/:token" element={<SharedBoardPage />} />
        <Route path="/people/shared/:token" element={<SharedPeoplePage />} />
      </Routes>
    );
  }

  if (!user) return <AuthPanel />;
  // Local mode (Android, serverless): mail only, straight to the provider.
  if (mode === "local") return <LocalShell />;

  return (
    <main className="app-shell">
      <Sidebar />
      <section className="workspace">
        <ZoneBanner />
        <Routes>
          <Route path="/" element={<CalendarPage />} />
          <Route path="/today" element={<TodayPage />} />
          <Route path="/tasks" element={<Gated><TasksPage /></Gated>} />
          <Route path="/notes" element={<Gated><NotesPage /></Gated>} />
          <Route path="/links" element={<Gated><LinksPage /></Gated>} />
          <Route path="/files" element={<Gated><FilesPage /></Gated>} />
          <Route path="/boards" element={<Gated><BoardsPage /></Gated>} />
          <Route path="/boards/:boardId" element={<Gated><BoardsPage /></Gated>} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/trash" element={<TrashPage />} />
          <Route path="/tags" element={<Gated><TagsPage /></Gated>} />
          <Route path="/people" element={<Gated><PeoplePage /></Gated>} />
          <Route path="/people/tree" element={<Gated><FamilyTreePage /></Gated>} />
          <Route path="/people/:id" element={<Gated><PersonProfilePage /></Gated>} />
          <Route path="/shopping" element={<Gated><ShoppingPage /></Gated>} />
          <Route path="/time" element={<Gated><TimePage /></Gated>} />
          <Route path="/mail" element={<Gated><MailPage /></Gated>} />
          <Route path="/github" element={<Gated><GitHubPage /></Gated>} />
          <Route path="/share" element={<ShareTarget />} />
          <Route path="/entry/:id" element={<EntryDeepLink />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </section>
      <BottomNav />
      <CommandPalette />
      <EntryEditor />
      <ContextMenu />
      <EventCard />
      <Toasts />
    </main>
  );
}

// Gated bounces a disabled module's route back to the calendar — the page
// isn't just hidden in nav, it's unreachable.
function Gated({ children }: { children: React.ReactNode }) {
  const settings = usePlanner((s) => s.settings);
  const location = useLocation();
  const mod = moduleForPath(location.pathname);
  if (mod && !moduleOn(settings, mod)) return <Navigate to="/" replace />;
  return <>{children}</>;
}

// ShareTarget receives PWA share_target GETs (?title=&text=&url=) and opens
// the editor prefilled — a shared URL becomes a link entry, text a note.
function ShareTarget() {
  const [params] = useSearchParams();
  const openCreate = useUi((s) => s.openCreate);
  const navigate = useNavigate();
  useEffect(() => {
    const url = params.get("url") ?? "";
    const title = params.get("title") ?? "";
    const text = params.get("text") ?? "";
    const isLink = /^https?:\/\//i.test(url || text);
    openCreate(undefined, undefined, {
      title: title || (isLink ? "" : text).slice(0, 120),
      linkUrl: url || (isLink ? text : ""),
      content: isLink ? "" : text,
      type: isLink ? "link" : "note",
    });
    navigate("/", { replace: true });
  }, [params, openCreate, navigate]);
  return null;
}

// EntryDeepLink resolves /entry/:id → opens the editor on that entry, then
// returns to the calendar. Entries may not be loaded yet on cold nav.
function EntryDeepLink() {
  const { id } = useParams();
  const entries = usePlanner((s) => s.entries);
  const openEdit = useUi((s) => s.openEdit);
  const navigate = useNavigate();
  const [attempted, setAttempted] = useState(false);
  useEffect(() => {
    if (!id) return;
    const entry = entries.find((e) => e.id === id);
    if (entry) {
      openEdit(entry);
      navigate("/", { replace: true });
    } else if (!attempted) {
      setAttempted(true);
      void usePlanner.getState().loadEntries({});
    } else {
      navigate("/", { replace: true }); // not found → calendar
    }
  }, [id, entries, attempted, openEdit, navigate]);
  return null;
}

export function App() {
  const { booted, bootstrap, settings } = usePlanner();
  const flushQueue = usePlanner((s) => s.flushQueue);

  useEffect(() => {
    void bootstrap();
  }, [bootstrap]);

  // Replay offline mutations whenever connectivity returns.
  useEffect(() => {
    const on = () => void flushQueue();
    window.addEventListener("online", on);
    return () => window.removeEventListener("online", on);
  }, [flushQueue]);

  // Resolve theme: light | dark | system (follows prefers-color-scheme live).
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme = settings.theme === "system" ? (media.matches ? "dark" : "light") : settings.theme;
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [settings.theme]);

  useEffect(() => {
    document.documentElement.dataset.accent = settings.accent;
  }, [settings.accent]);

  if (!booted) {
    return (
      <main className="splash">
        <div className="mark">
          <CalendarDays size={24} strokeWidth={2.2} />
        </div>
      </main>
    );
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/widget/today" element={<WidgetPage />} />
        <Route path="*" element={<Shell />} />
      </Routes>
    </BrowserRouter>
  );
}

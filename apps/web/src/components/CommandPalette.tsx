import type { Entry, Person } from "@cal/api-client";
import { AnimatePresence, motion } from "framer-motion";
import { ArrowRight, CalendarDays, Check, FolderOpen, Link2, Moon, Plus, Search, StickyNote, Sun, Kanban, UserRound } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { formatDayShort } from "../lib/date";
import { moduleOn, type ModuleKey } from "../lib/modules";
import { usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

interface Action {
  id: string;
  label: string;
  icon: React.ReactNode;
  module?: ModuleKey;
  run: () => void;
}

export function CommandPalette() {
  const open = useUi((state) => state.paletteOpen);
  const close = useUi((state) => state.closePalette);
  const openCreate = useUi((state) => state.openCreate);
  const openEdit = useUi((state) => state.openEdit);
  const selectDate = useUi((state) => state.selectDate);
  const setView = useUi((state) => state.setView);
  const goToday = useUi((state) => state.goToday);
  const api = usePlanner((state) => state.api);
  const toast = usePlanner((state) => state.toast);
  const settings = usePlanner((state) => state.settings);
  const updateSettings = usePlanner((state) => state.updateSettings);
  const selectedDate = useUi((state) => state.selectedDate);
  const navigate = useNavigate();

  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Entry[]>([]);
  const [fileResults, setFileResults] = useState<{ id: string; origName: string; name: string }[]>([]);
  const [boardResults, setBoardResults] = useState<{ id: string; name: string }[]>([]);
  const [peopleResults, setPeopleResults] = useState<Person[]>([]);
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const actions = useMemo<Action[]>(
    () => [
      {
        id: "new",
        label: "New entry",
        icon: <Plus size={15} />,
        run: () => {
          close();
          openCreate(selectedDate);
        },
      },
      {
        id: "today",
        label: "Go to today",
        icon: <CalendarDays size={15} />,
        run: () => {
          close();
          goToday();
          navigate("/today");
        },
      },
      {
        id: "calendar",
        label: "Go to calendar",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          navigate("/");
        },
      },
      {
        id: "tasks",
        label: "Go to tasks",
        icon: <ArrowRight size={15} />,
        module: "tasks",
        run: () => {
          close();
          navigate("/tasks");
        },
      },
      {
        id: "notes",
        label: "Go to notes",
        icon: <ArrowRight size={15} />,
        module: "notes",
        run: () => {
          close();
          navigate("/notes");
        },
      },
      {
        id: "links",
        label: "Go to links",
        icon: <ArrowRight size={15} />,
        module: "links",
        run: () => {
          close();
          navigate("/links");
        },
      },
      {
        id: "mail",
        label: "Go to mail",
        icon: <ArrowRight size={15} />,
        module: "mail",
        run: () => {
          close();
          navigate("/mail");
        },
      },
      {
        id: "month",
        label: "Month view",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          navigate("/");
          setView("month");
        },
      },
      {
        id: "week",
        label: "Week view",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          navigate("/");
          setView("week");
        },
      },
      {
        id: "day",
        label: "Day view",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          navigate("/");
          setView("day");
        },
      },
      {
        id: "boards",
        label: "Go to boards",
        icon: <ArrowRight size={15} />,
        module: "boards",
        run: () => {
          close();
          navigate("/boards");
        },
      },
      {
        id: "files",
        label: "Go to files",
        icon: <ArrowRight size={15} />,
        module: "files",
        run: () => {
          close();
          navigate("/files");
        },
      },
      {
        id: "tags",
        label: "Go to tags",
        icon: <ArrowRight size={15} />,
        module: "tags",
        run: () => {
          close();
          navigate("/tags");
        },
      },
      {
        id: "people",
        label: "Go to people",
        icon: <ArrowRight size={15} />,
        module: "people",
        run: () => {
          close();
          navigate("/people");
        },
      },
      {
        id: "time",
        label: "Go to time tracking",
        icon: <ArrowRight size={15} />,
        module: "time",
        run: () => {
          close();
          navigate("/time");
        },
      },
      {
        id: "github",
        label: "Go to GitHub",
        icon: <ArrowRight size={15} />,
        module: "github",
        run: () => {
          close();
          navigate("/github");
        },
      },
      {
        id: "trash",
        label: "Go to trash",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          navigate("/trash");
        },
      },
      {
        id: "settings",
        label: "Go to settings",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          navigate("/settings");
        },
      },
      {
        id: "timer",
        label: "Start focus timer",
        icon: <ArrowRight size={15} />,
        module: "time",
        run: () => {
          close();
          void api
            .startTimer()
            .then(() => toast("Timer started"))
            .catch((e) => toast(e instanceof Error ? e.message : "Could not start timer"));
        },
      },
      {
        id: "agenda",
        label: "Copy week agenda (markdown)",
        icon: <ArrowRight size={15} />,
        run: () => {
          close();
          void api
            .agenda(7)
            .then((md) => navigator.clipboard.writeText(md))
            .then(() => toast("Agenda copied"))
            .catch((e) => toast(e instanceof Error ? e.message : "Could not copy agenda"));
        },
      },
      {
        id: "theme",
        label: settings.theme === "dark" ? "Switch to light theme" : "Switch to dark theme",
        icon: settings.theme === "dark" ? <Sun size={15} /> : <Moon size={15} />,
        run: () => {
          close();
          void updateSettings({ ...settings, theme: settings.theme === "dark" ? "light" : "dark" });
        },
      },
    ],
    [close, goToday, openCreate, selectedDate, setView, settings, updateSettings, navigate, api, toast],
  );

  useEffect(() => {
    if (open) {
      setQuery("");
      setResults([]);
      setCursor(0);
      window.setTimeout(() => inputRef.current?.focus(), 20);
    }
  }, [open]);

  useEffect(() => {
    if (!open || !query.trim()) {
      setResults([]);
      return;
    }
    const timer = window.setTimeout(async () => {
      try {
        const out = await api.search(query.trim());
        setResults(out.entries);
        setFileResults(out.files);
        setBoardResults(out.boards ?? []);
        setPeopleResults(out.people ?? []);
      } catch {
        // Offline: fall back to the locally cached entries.
        const q = query.trim().toLowerCase();
        setResults(
          usePlanner
            .getState()
            .entries.filter(
              (entry) =>
                entry.title.toLowerCase().includes(q) ||
                (entry.content ?? "").toLowerCase().includes(q) ||
                entry.tags.some((tag) => tag.toLowerCase() === q),
            ),
        );
      }
    }, 180);
    return () => window.clearTimeout(timer);
  }, [query, open, api]);

  const filteredActions = useMemo(() => {
    const allowed = actions.filter((a) => !a.module || moduleOn(settings, a.module));
    const q = query.trim().toLowerCase();
    if (!q) return allowed;
    return allowed.filter((action) => action.label.toLowerCase().includes(q));
  }, [actions, query, settings]);

  const peopleOn = moduleOn(settings, "people");
  const items = useMemo(
    () => [
      ...filteredActions.map((action) => ({ kind: "action" as const, action })),
      ...results.map((entry) => ({ kind: "entry" as const, entry })),
      ...(peopleOn ? peopleResults.map((person) => ({ kind: "person" as const, person })) : []),
      ...fileResults.map((file) => ({ kind: "file" as const, file })),
      ...boardResults.map((board) => ({ kind: "board" as const, board })),
    ],
    [filteredActions, results, fileResults, boardResults, peopleResults, peopleOn],
  );

  useEffect(() => setCursor(0), [items.length]);

  useEffect(() => {
    listRef.current?.querySelector(".palette-item.active")?.scrollIntoView({ block: "nearest" });
  }, [cursor]);

  function choose(index: number) {
    const item = items[index];
    if (!item) return;
    if (item.kind === "action") {
      item.action.run();
    } else if (item.kind === "file") {
      close();
      window.open(api.assetUrl(`/api/files/${item.file.name}`), "_blank");
    } else if (item.kind === "board") {
      close();
      navigate(`/boards/${item.board.id}`);
    } else if (item.kind === "person") {
      close();
      navigate(`/people/${item.person.id}`);
    } else {
      close();
      if (item.entry.date) selectDate(item.entry.date);
      openEdit(item.entry);
    }
  }

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="scrim"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.12 }}
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) close();
          }}
        >
          <motion.div
            className="palette"
            role="dialog"
            aria-label="Command palette"
            initial={{ opacity: 0, scale: 0.98, y: -8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.98, y: -4 }}
            transition={{ type: "spring", duration: 0.3, bounce: 0 }}
          >
            <div className="palette-input">
              <Search size={16} style={{ color: "var(--text-3)", flex: "none" }} />
              <input
                ref={inputRef}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search entries or run a command…"
                onKeyDown={(event) => {
                  if (event.key === "ArrowDown") {
                    event.preventDefault();
                    setCursor((c) => Math.min(c + 1, items.length - 1));
                  } else if (event.key === "ArrowUp") {
                    event.preventDefault();
                    setCursor((c) => Math.max(c - 1, 0));
                  } else if (event.key === "Enter") {
                    event.preventDefault();
                    choose(cursor);
                  } else if (event.key === "Escape") {
                    close();
                  }
                }}
              />
            </div>
            <div className="palette-list" ref={listRef}>
              {filteredActions.length > 0 && <div className="palette-group">Actions</div>}
              {filteredActions.map((action) => {
                const index = items.findIndex((item) => item.kind === "action" && item.action.id === action.id);
                return (
                  <button
                    key={action.id}
                    type="button"
                    className={`palette-item ${cursor === index ? "active" : ""}`}
                    onMouseEnter={() => setCursor(index)}
                    onClick={() => choose(index)}
                  >
                    <span className="icon">{action.icon}</span>
                    {action.label}
                  </button>
                );
              })}
              {results.length > 0 && <div className="palette-group">Entries</div>}
              {results.map((entry) => {
                const index = filteredActions.length + results.indexOf(entry);
                const Icon = entry.type === "note" ? StickyNote : entry.type === "link" ? Link2 : Check;
                return (
                  <button
                    key={entry.id}
                    type="button"
                    className={`palette-item ${cursor === index ? "active" : ""}`}
                    onMouseEnter={() => setCursor(index)}
                    onClick={() => choose(index)}
                  >
                    <span className="swatch" style={{ "--swatch": `var(--c-${entry.color})` } as React.CSSProperties} />
                    {entry.title}
                    <span className="meta">
                      <Icon size={11} style={{ verticalAlign: "-1px" }} /> {formatDayShort(entry.date)}
                    </span>
                  </button>
                );
              })}
              {peopleOn && peopleResults.length > 0 && <div className="palette-group">People</div>}
              {peopleOn &&
                peopleResults.map((person) => {
                  const index = filteredActions.length + results.length + peopleResults.indexOf(person);
                  return (
                    <button
                      key={person.id}
                      type="button"
                      className={`palette-item ${cursor === index ? "active" : ""}`}
                      onMouseEnter={() => setCursor(index)}
                      onClick={() => choose(index)}
                    >
                      <span className="icon"><UserRound size={14} /></span>
                      {person.name}
                      {person.relation && <span className="meta">{person.relation}</span>}
                    </button>
                  );
                })}
              {fileResults.length > 0 && <div className="palette-group">Files</div>}
              {fileResults.map((file) => {
                const index = filteredActions.length + results.length + (peopleOn ? peopleResults.length : 0) + fileResults.indexOf(file);
                return (
                  <button
                    key={file.id}
                    type="button"
                    className={`palette-item ${cursor === index ? "active" : ""}`}
                    onMouseEnter={() => setCursor(index)}
                    onClick={() => choose(index)}
                  >
                    <span className="icon"><FolderOpen size={14} /></span>
                    {file.origName}
                  </button>
                );
              })}
              {boardResults.length > 0 && <div className="palette-group">Boards</div>}
              {boardResults.map((board) => {
                const index =
                  filteredActions.length + results.length + (peopleOn ? peopleResults.length : 0) + fileResults.length + boardResults.indexOf(board);
                return (
                  <button
                    key={board.id}
                    type="button"
                    className={`palette-item ${cursor === index ? "active" : ""}`}
                    onMouseEnter={() => setCursor(index)}
                    onClick={() => choose(index)}
                  >
                    <span className="icon"><Kanban size={14} /></span>
                    {board.name}
                  </button>
                );
              })}
              {query.trim() &&
                filteredActions.length === 0 &&
                results.length === 0 &&
                fileResults.length === 0 &&
                boardResults.length === 0 &&
                (!peopleOn || peopleResults.length === 0) && (
                  <div className="palette-empty">No matches for “{query.trim()}”</div>
                )}
            </div>
            <div className="palette-foot">
              <span><kbd>↑↓</kbd> navigate</span>
              <span><kbd>↵</kbd> select</span>
              <span><kbd>esc</kbd> close</span>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

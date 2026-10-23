// Boards — kanban project management. Cards are ordinary task entries
// (board_id/column_id/position): they complete via the editor, appear on the
// calendar, and in Today. Drag between columns, add cards inline.

import type { Board, BoardColumn, Entry } from "@cal/api-client";
import { Check, Link2, ListOrdered, Plus, Trash2, Kanban } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { PageHeader } from "../components/PageHeader";
import { formatDayShort, todayIso } from "../lib/date";
import { compareDates } from "../lib/entries";
import { reportErr, usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

export function BoardsPage() {
  const { boardId } = useParams();
  const api = usePlanner((s) => s.api);
  const toast = usePlanner((s) => s.toast);
  const openEdit = useUi((s) => s.openEdit);
  const createEntry = usePlanner((s) => s.createEntry);
  const updateEntry = usePlanner((s) => s.updateEntry);

  const [boards, setBoards] = useState<Board[]>([]);
  const [board, setBoard] = useState<Board>();
  const [columns, setColumns] = useState<BoardColumn[]>([]);
  const [cards, setCards] = useState<Entry[]>([]);
  const [newBoard, setNewBoard] = useState("");
  const [template, setTemplate] = useState("kanban");
  const [newCol, setNewCol] = useState("");
  const [newCard, setNewCard] = useState<Record<string, string>>({});
  const [dragId, setDragId] = useState<string>();
  const [overCol, setOverCol] = useState<string>();
  const [cardView, setCardView] = useState<"board" | "list">("board");
  const [viewReady, setViewReady] = useState(false);

  const loadBoards = useCallback(() => void api.boards().then(setBoards).catch(reportErr("Could not load boards")), [api]);
  const loadView = useCallback(
    (id: string) => {
      void api.boardView(id).then((v) => {
        setColumns(v.columns);
        setCards(v.cards);
      }).catch(reportErr("Could not load board")).finally(() => setViewReady(true));
    },
    [api],
  );

  useEffect(loadBoards, [loadBoards]);
  useEffect(() => {
    if (boardId) {
      setViewReady(false);
      void loadView(boardId);
      void api.boards().then((bs) => setBoard(bs.find((b) => b.id === boardId))).catch(reportErr("Could not load boards"));
    } else {
      setBoard(undefined);
      setColumns([]);
      setCards([]);
    }
  }, [boardId, api, loadView]);

  const byColumn = useMemo(() => {
    const m = new Map<string, Entry[]>();
    for (const card of cards) {
      const key = card.columnId ?? "";
      m.set(key, [...(m.get(key) ?? []), card]);
    }
    for (const list of m.values()) list.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    return m;
  }, [cards]);

  async function addBoard() {
    const name = newBoard.trim();
    if (!name) return;
    await api.createBoard(name, undefined, template).then((b) => {
      setBoards((bs) => [...bs, b]);
      setNewBoard("");
      toast("Board created");
    }).catch((e) => toast(e instanceof Error ? e.message : "Could not create board"));
  }

  async function addColumn() {
    const name = newCol.trim();
    if (!name || !boardId) return;
    await api.createColumn(boardId, name).then(() => {
      setNewCol("");
      loadView(boardId);
    }).catch((e) => toast(e instanceof Error ? e.message : "Could not create column"));
  }

  async function addCard(columnId: string) {
    const title = (newCard[columnId] ?? "").trim();
    if (!title || !boardId) return;
    const created = await createEntry({ title, type: "task", date: todayIso(), boardId, columnId });
    if (created) {
      setNewCard((m) => ({ ...m, [columnId]: "" }));
      loadView(boardId);
    }
  }

  // Fractional position: drop between neighbours without rewriting the column.
  function drop(cardId: string, columnId: string, beforeCard?: Entry) {
    const list = byColumn.get(columnId) ?? [];
    const idx = beforeCard ? list.findIndex((c) => c.id === beforeCard.id) : list.length;
    const prev = idx > 0 ? list[idx - 1].position : undefined;
    const next = idx < list.length ? list[idx].position : undefined;
    const position = prev !== undefined && next !== undefined ? (prev + next) / 2 : prev !== undefined ? prev + 1024 : next !== undefined ? next - 1024 : 1024;
    void api.moveCard(cardId, columnId, position).then(() => boardId && loadView(boardId)).catch((e) => toast(e instanceof Error ? e.message : "Move failed"));
  }

  if (!boardId) {
    return (
      <>
        <PageHeader title="Boards" sub="Kanban for projects — cards are real tasks, so they show up everywhere" />
        <div className="page-scroll">
          <div className="board-add">
            <input
              className="input"
              placeholder="New board name…"
              value={newBoard}
              onChange={(e) => setNewBoard(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && void addBoard()}
              aria-label="New board name"
            />
            <select
              className="select"
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
              aria-label="Board template"
            >
              <option value="blank">Blank</option>
              <option value="kanban">Kanban</option>
              <option value="sprint">Sprint</option>
              <option value="bugs">Bug tracker</option>
            </select>
            <button type="button" className="btn btn-primary" onClick={() => void addBoard()}>
              <Plus size={14} /> Create board
            </button>
          </div>
          {boards.length === 0 ? (
            <div className="empty-hint">
              <strong>No boards yet</strong>
              <span>A board groups tasks into columns — Todo, Doing, Done or your own flow.</span>
            </div>
          ) : (
            <div className="board-list">
              {boards.map((b) => (
                <Link key={b.id} to={`/boards/${b.id}`} className="board-tile">
                  <Kanban size={18} strokeWidth={1.8} />
                  <span className="board-name">{b.name}</span>
                  {b.total > 0 && (
                    <span className={`board-progress ${b.done === b.total ? "all" : ""}`}>
                      {b.done}/{b.total}
                    </span>
                  )}
                  {b.minutes > 0 && <span className="meta-chip">{b.minutes >= 60 ? `${Math.floor(b.minutes / 60)}h ${b.minutes % 60}m` : `${b.minutes}m`}</span>}
                  <button
                    type="button"
                    className="icon-btn"
                    aria-label={`Delete ${b.name}`}
                    onClick={(e) => {
                      e.preventDefault();
                      void api.deleteBoard(b.id).then(loadBoards).catch((e) => toast(e instanceof Error ? e.message : "Could not delete board"));
                    }}
                  >
                    <Trash2 size={14} />
                  </button>
                </Link>
              ))}
            </div>
          )}
        </div>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title={board?.name ?? "Board"}
        sub={board?.targetDate ? `target ${board.targetDate} — drag cards between columns` : "drag cards between columns — complete them like any task"}
      >
        <button
          type="button"
          className="btn btn-secondary btn-xs"
          onClick={() => {
            const d = window.prompt("Board description", board?.description ?? "");
            if (d === null) return;
            const t = window.prompt("Target date (YYYY-MM-DD, empty clears)", board?.targetDate ?? "");
            if (t === null) return;
            void api.updateBoard(boardId, { description: d, targetDate: t.trim() })
              .then(() => api.boards().then((bs) => setBoard(bs.find((b) => b.id === boardId))))
              .catch((e) => toast(e instanceof Error ? e.message : "Could not update board"));
          }}
        >
          Edit board
        </button>
        <button
          type="button"
          className="btn btn-secondary btn-xs"
          onClick={() =>
            void (async () => {
              try {
                const edit = window.confirm("Editable link? OK = viewers can move cards, Cancel = view only");
                const { shareToken } = await api.shareBoard(boardId, true, edit);
                const url = `${api.remote || window.location.origin}/board/${shareToken}`;
                try {
                  await navigator.clipboard.writeText(url);
                  toast(edit ? "Editable link copied — viewers can move cards" : "Public link copied — view only");
                } catch {
                  toast(`Share link: ${url}`);
                }
              } catch (e) {
                toast(e instanceof Error ? e.message : "Could not create share link");
              }
            })()
          }
        >
          <Link2 size={12} /> Share
        </button>
        <button
          type="button"
          className={`btn btn-secondary btn-xs ${cardView === "list" ? "on" : ""}`}
          aria-pressed={cardView === "list"}
          onClick={() => setCardView(cardView === "list" ? "board" : "list")}
        >
          {cardView === "list" ? "Board view" : "List view"}
        </button>
        <Link to="/boards" className="btn btn-secondary btn-xs">All boards</Link>
      </PageHeader>
      {board?.description && <p className="board-desc">{board.description}</p>}
      {cardView === "list" ? (
        <div className="page-scroll" style={{ paddingTop: 0 }}>
          {columns.map((col) => {
            const items = (byColumn.get(col.id) ?? []).sort((a, b) => compareDates(a.date, b.date));
            return (
              <section key={col.id} className="panel" style={{ marginBottom: 10 }}>
                <h3>
                  {col.name}
                  <span className="kanban-count" style={{ marginLeft: 8 }}>{items.length}</span>
                </h3>
                <ul className="check-list">
                  {items.map((card) => (
                    <li key={card.id} className={card.completed ? "done" : ""}>
                      <button
                        type="button"
                        className="tickbox"
                        aria-label={card.completed ? "Reopen" : "Complete"}
                        onClick={() => void updateEntry(card.id, { completed: !card.completed })}
                      >
                        {card.completed && <Check size={12} strokeWidth={3} />}
                      </button>
                      <button type="button" className="row-title" onClick={() => openEdit(card)}>
                        {card.title}
                      </button>
                      {card.blockedBy && <span className="meta-chip overdue">blocked</span>}
                      {card.date && <span className="meta-chip">{formatDayShort(card.date)}</span>}
                      {card.tags.map((t) => (
                        <span key={t} className="meta-chip">{t}</span>
                      ))}
                    </li>
                  ))}
                  {items.length === 0 && <li className="panel-empty">Empty column.</li>}
                </ul>
              </section>
            );
          })}
        </div>
      ) : (
      <div className="kanban">
        {columns.length === 0 && (
          <p className="panel-empty" style={{ padding: 12 }}>{viewReady ? "No columns — add one on the right." : "Loading…"}</p>
        )}
        {columns.map((col) => (
          <section
            key={col.id}
            className={`kanban-col ${overCol === col.id ? "over" : ""}`}
            onDragOver={(e) => {
              e.preventDefault();
              setOverCol(col.id);
            }}
            onDragLeave={() => setOverCol((c) => (c === col.id ? undefined : c))}
            onDrop={(e) => {
              e.preventDefault();
              setOverCol(undefined);
              if (dragId) drop(dragId, col.id);
              setDragId(undefined);
            }}
            aria-label={col.name}
          >
            <header className="kanban-head">
              <span className="kanban-name">{col.name}</span>
              <span
                className={`kanban-count ${col.wipLimit && (byColumn.get(col.id) ?? []).length > col.wipLimit ? "over" : ""}`}
                title={col.wipLimit ? `WIP limit ${col.wipLimit}` : undefined}
              >
                {(byColumn.get(col.id) ?? []).length}
                {col.wipLimit ? `/${col.wipLimit}` : ""}
              </span>
              <button
                type="button"
                className="icon-btn"
                aria-label={`WIP limit for ${col.name}`}
                title="Set WIP limit (0 clears)"
                onClick={() => {
                  const v = window.prompt(`WIP limit for "${col.name}" (empty clears)`, String(col.wipLimit ?? ""));
                  if (v === null) return;
                  const n = v.trim() === "" ? 0 : parseInt(v, 10);
                  if (Number.isNaN(n) || n < 0) return;
                  void api.updateColumn(col.id, { wipLimit: n }).then(() => boardId && loadView(boardId)).catch((e) => toast(e instanceof Error ? e.message : "Could not set limit"));
                }}
              >
                <ListOrdered size={12} />
              </button>
              <button
                type="button"
                className="icon-btn"
                aria-label={`Delete column ${col.name}`}
                onClick={() => void api.deleteColumn(col.id).then(() => boardId && loadView(boardId)).catch((e) => toast(e instanceof Error ? e.message : "Could not delete column"))}
              >
                <Trash2 size={12} />
              </button>
            </header>
            <div className="kanban-cards">
              {(byColumn.get(col.id) ?? []).map((card) => (
                <div
                  key={card.id}
                  className={`kanban-card color-${card.color} ${card.completed ? "done" : ""}`}
                  draggable
                  onDragStart={() => setDragId(card.id)}
                  onDragEnd={() => setDragId(undefined)}
                  onDragOver={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                  }}
                  onDrop={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    if (dragId && dragId !== card.id) drop(dragId, col.id, card);
                    setDragId(undefined);
                  }}
                >
                  <button type="button" className="kanban-card-title" onClick={() => openEdit(card)}>
                    {card.title}
                  </button>
                  <div className="kanban-card-meta">
                    <CardDue date={card.date} done={card.completed} />
                    <CardChecklist content={card.content} />
                    {card.blockedBy && <span className="meta-chip overdue">blocked</span>}
                    {card.tags.slice(0, 3).map((t) => (
                      <span key={t} className="stream-tag">#{t}</span>
                    ))}
                    {card.completed && <Check size={12} className="kanban-done" aria-label="Done" />}
                  </div>
                </div>
              ))}
            </div>
            <div className="kanban-add">
              <input
                className="kanban-add-input"
                placeholder="+ card"
                value={newCard[col.id] ?? ""}
                onChange={(e) => setNewCard((m) => ({ ...m, [col.id]: e.target.value }))}
                onKeyDown={(e) => e.key === "Enter" && void addCard(col.id)}
                aria-label={`Add card to ${col.name}`}
              />
            </div>
          </section>
        ))}
        <div className="kanban-col kanban-col-new">
          <input
            className="input"
            placeholder="+ Column"
            value={newCol}
            onChange={(e) => setNewCol(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void addColumn()}
            aria-label="New column name"
          />
        </div>
      </div>
      )}
    </>
  );
}

// CardDue — date chip on the card face; red when past due and not done.
function CardDue({ date, done }: { date?: string; done: boolean }) {
  if (!date) return null;
  const overdue = !done && date < todayIso();
  const today = date === todayIso();
  return (
    <span className={`card-due ${overdue ? "over" : ""} ${today ? "today" : ""}`}>
      {formatDayShort(date)}
    </span>
  );
}

// CardChecklist — "2/5" chip counting `- [ ]` / `- [x]` lines in content.
function CardChecklist({ content }: { content?: string }) {
  if (!content) return null;
  const total = (content.match(/^\s*[-*]\s+\[[ x]\]/gim) ?? []).length;
  const done = (content.match(/^\s*[-*]\s+\[x\]/gim) ?? []).length;
  if (!total) return null;
  return (
    <span className={`card-checks ${done === total ? "all" : ""}`}>
      <Check size={10} /> {done}/{total}
    </span>
  );
}

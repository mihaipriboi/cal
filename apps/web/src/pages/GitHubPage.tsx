// GitHub — open issues/PRs involving you, one-click import to a board card.

import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { GitPullRequest, Import, RefreshCw } from "lucide-react";
import { PageHeader } from "../components/PageHeader";
import { reportErr, usePlanner } from "../stores/planner";

interface GHItem {
  number: number;
  title: string;
  state: string;
  url: string;
  repo: string;
  isPR: boolean;
  labels: string[];
}

export function GitHubPage() {
  const api = usePlanner((s) => s.api);
  const settings = usePlanner((s) => s.settings);
  const [boards, setBoards] = useState<{ id: string; name: string }[]>([]);
  const toast = usePlanner((s) => s.toast);
  const [items, setItems] = useState<GHItem[]>([]);
  const [activity, setActivity] = useState<{ login: string; eventsThisWeek: number } | null>(null);
  const [err, setErr] = useState(false);
  const [loading, setLoading] = useState(false);

  const load = () => {
    void api.boards().then(setBoards).catch(reportErr("Could not load boards"));
    if (!settings.githubConnected) { setErr(true); return; }
    setLoading(true);
    void api.githubInbox()
      .then((x) => { setItems(x); setErr(false); })
      .catch(() => setErr(true))
      .finally(() => setLoading(false));
    void api.githubActivity().then(setActivity).catch(() => {});
  };
  useEffect(load, [api]);

  const byRepo = useMemo(() => {
    const m = new Map<string, GHItem[]>();
    for (const it of items) m.set(it.repo, [...(m.get(it.repo) ?? []), it]);
    return [...m.entries()].sort((a, b) => b[1].length - a[1].length);
  }, [items]);

  return (
    <>
      <PageHeader
        title="GitHub"
        sub={activity ? `${activity.login} — ${activity.eventsThisWeek} events this week` : "issue & PR inbox"}
      >
        <button type="button" className="btn btn-secondary btn-xs" onClick={load} disabled={loading}>
          <RefreshCw size={12} /> Refresh
        </button>
      </PageHeader>
      <div className="page-scroll">
        {err ? (
          <div className="empty-hint">
            <strong>No GitHub token</strong>
            <span>Add a personal access token under Integrations, then refresh.</span>
            <Link className="btn btn-primary" to="/settings" style={{ marginTop: 10 }}>
              Open Settings
            </Link>
          </div>
        ) : byRepo.length === 0 ? (
          <div className="empty-hint">
            <strong>Inbox zero</strong>
            <span>No open issues or PRs involve you right now.</span>
          </div>
        ) : (
          byRepo.map(([repo, repoItems]) => (
            <section key={repo} className="panel" style={{ marginBottom: 10 }}>
              <h3>
                <GitPullRequest size={13} style={{ verticalAlign: "-2px", marginRight: 5 }} />
                {repo}
                <span className="kanban-count" style={{ marginLeft: 8 }}>{repoItems.length}</span>
              </h3>
              <ul className="trash-list" style={{ padding: 0 }}>
                {repoItems.map((it) => (
                  <li key={it.url} className="trash-row">
                    {it.isPR ? <GitPullRequest size={14} style={{ color: "var(--accent)" }} /> : <span className="gh-num">#{it.number}</span>}
                    <a className="trash-title tag-open" href={it.url} target="_blank" rel="noopener noreferrer">
                      {it.title}
                    </a>
                    {it.labels.slice(0, 2).map((l) => (
                      <span key={l} className="meta-chip">{l}</span>
                    ))}
                    {boards.length > 0 && (
                      <button
                        type="button"
                        className="btn btn-secondary btn-xs"
                        onClick={() =>
                          void api.githubImport(it.url, boards[0].id)
                            .then(() => toast(`Imported to ${boards[0].name}`))
                            .catch(() => toast("Import failed"))
                        }
                      >
                        <Import size={11} /> To {boards[0].name}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          ))
        )}
      </div>
    </>
  );
}

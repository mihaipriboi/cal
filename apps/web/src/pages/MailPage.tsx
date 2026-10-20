// Mail — IMAP/SMTP client. Left rail: accounts + mailboxes. Middle:
// envelope list. Right: reader/compose. Briefkescht-shaped triage —
// read, reply, flag, bin — without the bloat.
// Transport is pluggable: server-proxied over /api/mail in server mode,
// straight to the provider via the CalMail plugin in local (Android) mode.

import type { MailAccount, MailMessage, MailSummary } from "@cal/api-client";
import { Inbox, Mail, Paperclip, PenSquare, Plus, RefreshCw, Send, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { mailBackend, type MailBackend } from "../lib/mail";
import { hasRemoteContent, MAIL_FRAME_SANDBOX, mailFrameDoc } from "../lib/mailframe";
import { reportErr, usePlanner } from "../stores/planner";

export function MailPage() {
  const api = usePlanner((s) => s.api);
  const mode = usePlanner((s) => s.mode);
  const mail = useMemo<MailBackend>(() => mailBackend(api), [api, mode]);
  const toast = usePlanner((s) => s.toast);
  const [accounts, setAccounts] = useState<MailAccount[]>([]);
  const [account, setAccount] = useState<MailAccount | null>(null);
  const [mailboxes, setMailboxes] = useState<{ name: string }[]>([]);
  const [mailbox, setMailbox] = useState("INBOX");
  const [list, setList] = useState<MailSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [reading, setReading] = useState<MailMessage | null>(null);
  const [readingUid, setReadingUid] = useState<number | null>(null);
  const [compose, setCompose] = useState(false);
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const loadAccounts = useCallback(() => {
    void mail.accounts().then(setAccounts).catch((e) => {
      setAccounts([]);
      reportErr("Could not load mail accounts")(e);
    });
  }, [mail]);
  useEffect(loadAccounts, [loadAccounts]);

  const pickAccount = useCallback(
    (a: MailAccount) => {
      setAccount(a);
      setMailbox("INBOX");
      setReading(null);
      setReadingUid(null);
      void mail.mailboxes(a.id).then(setMailboxes).catch((e) => {
        setMailboxes([]);
        reportErr("Could not load mailboxes")(e);
      });
    },
    [mail],
  );

  const loadMessages = useCallback(
    (acct: MailAccount, box: string) => {
      setBusy(true);
      setErr("");
      void mail
        .messages(acct.id, box)
        .then((r) => {
          setList(r.messages);
          setTotal(r.total);
        })
        .catch((e: Error) => setErr(e.message))
        .finally(() => setBusy(false));
    },
    [mail],
  );

  useEffect(() => {
    if (account) loadMessages(account, mailbox);
  }, [account, mailbox, loadMessages]);

  async function openMessage(uid: number) {
    if (!account) return;
    setReadingUid(uid);
    setReading(null);
    try {
      const msg = await mail.message(account.id, uid, mailbox);
      setReading(msg);
      // Mark seen + reflect locally.
      void mail.setSeen(account.id, uid, true, mailbox).catch((e) => toast(e instanceof Error ? e.message : "Could not mark read"));
      setList((l) => l.map((m) => (m.uid === uid ? { ...m, seen: true } : m)));
    } catch {
      setErr("Could not open message");
      setReadingUid(null);
    }
  }

  async function removeMessage(uid: number) {
    if (!account) return;
    try {
      await mail.remove(account.id, uid, mailbox);
    } catch (e) {
      toast(e instanceof Error ? e.message : "Delete failed");
      return;
    }
    setList((l) => l.filter((m) => m.uid !== uid));
    if (readingUid === uid) {
      setReading(null);
      setReadingUid(null);
    }
  }

  if (accounts.length === 0 && !adding) {
    return (
      <>
        <PageHeader title="Mail" sub="connect an account to begin" />
        <div className="page-scroll">
          <div className="empty-hint">
            <Inbox size={28} style={{ color: "var(--text-3)" }} />
            <strong>No mail accounts</strong>
            <span>IMAP/SMTP — works with Gmail, Fastmail, Dovecot, anything.</span>
            <button type="button" className="btn btn-primary" onClick={() => setAdding(true)}>
              <Plus size={14} /> Add account
            </button>
          </div>
        </div>
      </>
    );
  }

  return (
    <>
      <PageHeader title="Mail" sub={account ? `${account.email} · ${mailbox}` : "choose an account"}>
        {account && (
          <>
            <button type="button" className="btn btn-secondary btn-xs" onClick={() => loadMessages(account, mailbox)} disabled={busy}>
              <RefreshCw size={12} /> Refresh
            </button>
            <button type="button" className="btn btn-primary btn-xs" onClick={() => setCompose(true)}>
              <PenSquare size={12} /> Compose
            </button>
          </>
        )}
        {!account && accounts.length > 0 && (
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => setAdding(true)}>
            <Plus size={12} /> Account
          </button>
        )}
      </PageHeader>
      <div className="page-scroll mail-layout">
        <aside className="mail-rail">
          <div className="side-label">Accounts</div>
          {accounts.map((a) => (
            <button
              key={a.id}
              type="button"
              className={`filter-item ${account?.id === a.id ? "on" : ""}`}
              onClick={() => pickAccount(a)}
            >
              <Mail size={13} />
              <span className="mail-acct-name">{a.name || a.email}</span>
            </button>
          ))}
          <button type="button" className="filter-item" onClick={() => setAdding(true)}>
            <Plus size={13} /> Add account
          </button>
          {account && mailboxes.length > 0 && (
            <>
              <div className="side-label" style={{ marginTop: 12 }}>Mailboxes</div>
              {mailboxes.slice(0, 12).map((b) => (
                <button
                  key={b.name}
                  type="button"
                  className={`filter-item ${mailbox === b.name ? "on" : ""}`}
                  onClick={() => {
                    setMailbox(b.name);
                    setReading(null);
                  }}
                >
                  {b.name}
                </button>
              ))}
            </>
          )}
        </aside>

        <section className="mail-list">
          {err && <p className="panel-note" style={{ color: "var(--c-red)" }}>{err}</p>}
          {!account && <p className="panel-empty">Select an account on the left.</p>}
          {account && busy && list.length === 0 && <p className="panel-empty">Loading…</p>}
          {account && !busy && list.length === 0 && !err && <p className="panel-empty">Empty mailbox.</p>}
          <ul className="mail-items">
            {list.map((m) => (
              <li key={m.uid}>
                <button
                  type="button"
                  className={`mail-item ${m.seen ? "seen" : ""} ${readingUid === m.uid ? "active" : ""}`}
                  onClick={() => void openMessage(m.uid)}
                >
                  <span className="mail-from">{m.from || "(unknown)"}</span>
                  <span className="mail-subject">{m.subject || "(no subject)"}</span>
                  <span className="mail-date">
                    {new Date(m.date).toLocaleDateString(undefined, { month: "short", day: "numeric" })}
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {account && total > list.length && <p className="panel-note">{total} messages total</p>}
        </section>

        <section className="mail-reader">
          {readingUid && !reading && <p className="panel-empty">Opening…</p>}
          {!readingUid && <p className="panel-empty">Select a message.</p>}
          {reading && (
            <div className="mail-body">
              <h3>{reading.subject || "(no subject)"}</h3>
              <p className="panel-note">
                {reading.from} → {reading.to.join(", ")}
              </p>
              <div className="mail-actions">
                <button type="button" className="btn btn-secondary btn-xs" onClick={() => setCompose(true)}>
                  <PenSquare size={12} /> Reply
                </button>
                <button
                  type="button"
                  className="btn btn-secondary btn-xs"
                  onClick={() =>
                    account &&
                    void mail
                      .setSeen(account.id, reading.uid, false, mailbox)
                      .then(() => setList((l) => l.map((m) => (m.uid === reading.uid ? { ...m, seen: false } : m))))
                      .catch((e) => toast(e instanceof Error ? e.message : "Could not flag"))
                  }
                >
                  Unread
                </button>
                <button
                  type="button"
                  className="btn btn-secondary btn-xs danger"
                  onClick={() => void removeMessage(reading.uid)}
                >
                  <Trash2 size={12} /> Delete
                </button>
              </div>
              {reading.text ? (
                <pre className="mail-text">{reading.text}</pre>
              ) : (
                <MailHtml key={reading.uid} html={reading.html} />
              )}
            </div>
          )}
        </section>
      </div>

      {adding && <AccountDialog onClose={() => setAdding(false)} onSaved={() => { setAdding(false); loadAccounts(); }} />}
      {compose && account && (
        <ComposeDialog
          account={account}
          replyTo={reading}
          onClose={() => setCompose(false)}
          onSent={() => {
            setCompose(false);
            toast("Sent");
          }}
        />
      )}
    </>
  );
}

// HTML mail renders in a sandboxed iframe (see lib/mailframe). Remote images
// load only after the reader asks; keyed by uid so each message starts blocked.
function MailHtml({ html }: { html: string }) {
  const [allowRemote, setAllowRemote] = useState(false);
  const remote = useMemo(() => hasRemoteContent(html), [html]);
  return (
    <>
      {remote && !allowRemote && (
        <p className="panel-note mail-remote">
          Remote images are blocked.{" "}
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => setAllowRemote(true)}>
            Load remote images
          </button>
        </p>
      )}
      <iframe
        className="mail-frame"
        title="Message"
        sandbox={MAIL_FRAME_SANDBOX}
        referrerPolicy="no-referrer"
        srcDoc={mailFrameDoc(html, allowRemote)}
      />
    </>
  );
}

function AccountDialog({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const api = usePlanner((s) => s.api);
  const mail = mailBackend(api);
  const [f, setF] = useState({ name: "", email: "", imapHost: "", imapPort: "", smtpHost: "", smtpPort: "", username: "", password: "", insecure: false });
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });
  const port = (v: string) => {
    const n = parseInt(v, 10);
    return n > 0 && n < 65536 ? n : undefined;
  };

  async function save() {
    setBusy(true);
    setErr("");
    let savedErr = "";
    try {
      const acct = await mail.addAccount({
        name: f.name, email: f.email,
        imapHost: f.imapHost, imapPort: port(f.imapPort),
        smtpHost: f.smtpHost, smtpPort: port(f.smtpPort),
        username: f.username, password: f.password, insecure: f.insecure,
      });
      // Verify the credentials actually log in before declaring victory.
      // The check covers IMAP and SMTP — keep the message generic.
      await mail.testAccount(acct.id).catch((e: Error) => {
        savedErr = `Saved, but the login check failed: ${e.message}`;
        throw e;
      });
      onSaved();
    } catch (e) {
      setErr(savedErr || (e instanceof Error ? e.message : "Failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="scrim" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="palette mail-dialog" role="dialog" aria-label="Add mail account">
        <h3>Add mail account</h3>
        <label className="field"><span>Name</span><input value={f.name} onChange={set("name")} placeholder="Work" /></label>
        <label className="field"><span>Email</span><input type="email" value={f.email} onChange={set("email")} placeholder="you@example.com" required /></label>
        <div className="field-row">
          <label className="field"><span>IMAP host</span><input value={f.imapHost} onChange={set("imapHost")} placeholder="imap.example.com" required /></label>
          <label className="field"><span>Port</span><input type="number" inputMode="numeric" min={1} max={65535} value={f.imapPort} onChange={set("imapPort")} placeholder="993" /></label>
        </div>
        <div className="field-row">
          <label className="field"><span>SMTP host</span><input value={f.smtpHost} onChange={set("smtpHost")} placeholder="smtp.example.com" required /></label>
          <label className="field"><span>Port</span><input type="number" inputMode="numeric" min={1} max={65535} value={f.smtpPort} onChange={set("smtpPort")} placeholder="465" /></label>
        </div>
        <label className="field"><span>Username (blank = email)</span><input value={f.username} onChange={set("username")} /></label>
        <label className="field"><span>Password / app token</span><input type="password" value={f.password} onChange={set("password")} required /></label>
        <label className="field field-check">
          <input type="checkbox" checked={f.insecure} onChange={(e) => setF({ ...f, insecure: e.target.checked })} />
          <span>Allow self-signed certificates (unsafe — for self-hosted mail)</span>
        </label>
        {err && <p className="panel-note" style={{ color: "var(--c-red)" }}>{err}</p>}
        <div className="mail-dialog-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn-primary" disabled={busy || !f.email || !f.imapHost || !f.password} onClick={() => void save()}>
            {busy ? "Verifying…" : "Connect"}
          </button>
        </div>
      </div>
    </div>
  );
}

// bareAddr pulls the addr-spec out of a display-name header value —
// "Jane <jane@x>" → "jane@x". The server's RCPT list takes the raw To field,
// so a display name there would send a malformed envelope.
function bareAddr(from?: string): string {
  if (!from) return "";
  return from.match(/<([^>]+)>/)?.[1] ?? from;
}

function ComposeDialog({ account, replyTo, onClose, onSent }: { account: MailAccount; replyTo: MailMessage | null; onClose: () => void; onSent: () => void }) {
  const api = usePlanner((s) => s.api);
  const mail = mailBackend(api);
  const [to, setTo] = useState(bareAddr(replyTo?.from));
  const [subject, setSubject] = useState(replyTo ? `Re: ${replyTo.subject}` : "");
  const [text, setText] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  // Picked files stay File objects; the backend decides transport — upload to
  // /files in server mode, base64 straight into the plugin in local mode.
  function attach(list: FileList | null) {
    if (!list?.length) return;
    setFiles((cur) => [...cur, ...Array.from(list)].slice(0, 10));
    if (fileRef.current) fileRef.current.value = "";
  }

  async function send() {
    setBusy(true);
    setErr("");
    try {
      await mail.send(account.id, { to, subject, text, files });
      onSent();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Send failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="scrim" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="palette mail-dialog" role="dialog" aria-label="Compose">
        <h3>{replyTo ? "Reply" : "Compose"}</h3>
        <label className="field"><span>To</span><input value={to} onChange={(e) => setTo(e.target.value)} required /></label>
        <label className="field"><span>Subject</span><input value={subject} onChange={(e) => setSubject(e.target.value)} /></label>
        <textarea className="mail-compose" rows={10} value={text} onChange={(e) => setText(e.target.value)} placeholder="Write…" />
        {files.length > 0 && (
          <div className="mail-attachments">
            {files.map((f, i) => (
              <span key={`${f.name}-${i}`} className="chip">
                <Paperclip size={11} /> {f.name}
                <button type="button" className="chip-x" aria-label={`Remove ${f.name}`} onClick={() => setFiles((cur) => cur.filter((x) => x !== f))}>×</button>
              </span>
            ))}
          </div>
        )}
        <input ref={fileRef} type="file" multiple hidden onChange={(e) => attach(e.target.files)} />
        {err && <p className="panel-note" style={{ color: "var(--c-red)" }}>{err}</p>}
        <div className="mail-dialog-actions">
          <button type="button" className="btn btn-secondary" onClick={() => fileRef.current?.click()}>
            <Paperclip size={13} /> Attach
          </button>
          <span style={{ flex: 1 }} />
          <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn-primary" disabled={busy || !to} onClick={() => void send()}>
            <Send size={13} /> {busy ? "Sending…" : "Send"}
          </button>
        </div>
      </div>
    </div>
  );
}

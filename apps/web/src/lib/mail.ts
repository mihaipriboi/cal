import type { CalApi, MailAccount, MailMessage, MailSummary } from "@cal/api-client";
import { registerPlugin } from "@capacitor/core";
import { isLocalMode } from "./local";

// MailBackend — one mail surface, two transports. `httpMail` proxies through
// the Cal API (server dials IMAP/SMTP, creds in Postgres); `nativeMail` calls
// the Android CalMail plugin (device dials the provider directly, creds in
// Keystore-encrypted prefs). MailPage doesn't care which it holds.
export interface MailAccountInput {
  name?: string;
  email: string;
  imapHost: string;
  imapPort?: number;
  smtpHost: string;
  smtpPort?: number;
  username?: string;
  password: string;
  /** Trust invalid/self-signed certs — self-hosted mail servers. */
  insecure?: boolean;
}

export interface MailSendInput {
  to: string;
  cc?: string;
  subject: string;
  text: string;
  files: File[];
}

export interface MailBackend {
  accounts(): Promise<MailAccount[]>;
  addAccount(input: MailAccountInput): Promise<MailAccount>;
  removeAccount(id: string): Promise<void>;
  testAccount(id: string): Promise<void>;
  mailboxes(id: string): Promise<{ name: string; delimiter?: string }[]>;
  messages(id: string, mailbox: string, page?: number): Promise<{ total: number; messages: MailSummary[] }>;
  message(id: string, uid: number, mailbox: string): Promise<MailMessage>;
  setSeen(id: string, uid: number, seen: boolean, mailbox: string): Promise<void>;
  remove(id: string, uid: number, mailbox: string): Promise<void>;
  send(id: string, input: MailSendInput): Promise<void>;
}

export function httpMail(api: CalApi): MailBackend {
  return {
    accounts: () => api.mailAccounts(),
    addAccount: (input) => api.createMailAccount(input),
    removeAccount: (id) => api.deleteMailAccount(id),
    testAccount: async (id) => {
      await api.testMailAccount(id);
    },
    mailboxes: (id) => api.mailMailboxes(id),
    messages: (id, mailbox, page = 0) => api.mailMessages(id, mailbox, page),
    message: (id, uid, mailbox) => api.mailMessage(id, uid, mailbox),
    setSeen: (id, uid, seen, mailbox) => api.mailFlag(id, uid, seen, mailbox),
    remove: (id, uid, mailbox) => api.mailDelete(id, uid, mailbox),
    // Server path attaches stored /files names — upload first, send second.
    async send(id, input) {
      const names: string[] = [];
      for (const file of input.files) {
        const out = await api.upload(file);
        names.push(out.name);
      }
      await api.mailSend(id, { to: input.to, cc: input.cc, subject: input.subject, text: input.text, attachments: names });
    },
  };
}

// Capacitor-side plugin contract — mirrors the /api/mail/* surface plus
// exportAccounts for the connect-a-server import.
interface CalMailPlugin {
  listAccounts(): Promise<{ accounts: MailAccount[] }>;
  addAccount(input: MailAccountInput): Promise<{ account: MailAccount }>;
  removeAccount(input: { id: string }): Promise<void>;
  testAccount(input: { id: string }): Promise<{ ok: boolean }>;
  /** Native confirmation dialog first; rejects with "cancelled" if declined. */
  exportAccounts(input: { destination: string }): Promise<{ accounts: (MailAccountInput & { id: string })[] }>;
  mailboxes(input: { id: string }): Promise<{ mailboxes: { name: string; delimiter: string }[] }>;
  messages(input: { id: string; mailbox: string; page: number }): Promise<{ total: number; messages: MailSummary[] }>;
  message(input: { id: string; mailbox: string; uid: number }): Promise<MailMessage>;
  setSeen(input: { id: string; mailbox: string; uid: number; seen: boolean }): Promise<void>;
  deleteMessage(input: { id: string; mailbox: string; uid: number }): Promise<void>;
  send(input: {
    id: string;
    to: string;
    cc?: string;
    subject: string;
    text: string;
    attachments: { name: string; mime: string; data: string }[];
  }): Promise<void>;
}

const plugin = registerPlugin<CalMailPlugin>("CalMail");

export function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result as string;
      resolve(result.slice(result.indexOf(",") + 1));
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

export interface NativeMailBackend extends MailBackend {
  exportAccounts(destination: string): Promise<(MailAccountInput & { id: string })[]>;
}

export const nativeMail: NativeMailBackend = {
  async accounts() {
    return (await plugin.listAccounts()).accounts;
  },
  async addAccount(input) {
    return (await plugin.addAccount(input)).account;
  },
  removeAccount: (id) => plugin.removeAccount({ id }),
  testAccount: async (id) => {
    await plugin.testAccount({ id });
  },
  exportAccounts: async (destination) => (await plugin.exportAccounts({ destination })).accounts,
  mailboxes: async (id) => (await plugin.mailboxes({ id })).mailboxes,
  messages: (id, mailbox, page = 0) => plugin.messages({ id, mailbox, page }),
  message: (id, uid, mailbox) => plugin.message({ id, mailbox, uid }),
  setSeen: (id, uid, seen, mailbox) => plugin.setSeen({ id, mailbox, uid, seen }),
  remove: (id, uid, mailbox) => plugin.deleteMessage({ id, mailbox, uid }),
  async send(id, input) {
    const attachments = await Promise.all(
      input.files.map(async (file) => ({
        name: file.name,
        mime: file.type || "application/octet-stream",
        data: await fileToBase64(file),
      })),
    );
    await plugin.send({ id, to: input.to, cc: input.cc, subject: input.subject, text: input.text, attachments });
  },
};

export function mailBackend(api: CalApi): MailBackend {
  return isLocalMode() ? nativeMail : httpMail(api);
}

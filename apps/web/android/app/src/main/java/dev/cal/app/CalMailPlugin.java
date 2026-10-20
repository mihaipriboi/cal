package dev.cal.app;

import android.content.Context;
import androidx.appcompat.app.AlertDialog;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;

import com.getcapacitor.JSArray;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.security.SecureRandom;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.Properties;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import javax.mail.Address;
import javax.mail.FetchProfile;
import javax.mail.Flags;
import javax.mail.Folder;
import javax.mail.Message;
import javax.mail.MessagingException;
import javax.mail.Multipart;
import javax.mail.Part;
import javax.mail.Session;
import javax.mail.Store;
import javax.mail.Transport;
import javax.mail.UIDFolder;
import javax.mail.internet.InternetAddress;
import javax.mail.internet.MimeBodyPart;
import javax.mail.internet.MimeMessage;
import javax.mail.internet.MimeMultipart;
import javax.mail.util.ByteArrayDataSource;

import com.sun.mail.imap.IMAPFolder;

/**
 * CalMail — on-device IMAP/SMTP for local mode. The web bundle can't open TCP
 * sockets, so the plugin does it with Jakarta Mail for Android. Accounts live
 * in SharedPreferences as one AES-256-GCM blob keyed from AndroidKeyStore —
 * same at-rest story as the server's credential store, minus the server.
 *
 * Method surface mirrors /api/mail/* so the frontend's MailBackend is a pure
 * transport swap.
 */
@CapacitorPlugin(name = "CalMail")
public class CalMailPlugin extends Plugin {

    private static final int BODY_CAP = 2 << 20; // mirror server: 2MB

    private final ExecutorService io = Executors.newCachedThreadPool();
    private AccountStore store;

    private interface Job {
        JSObject go() throws Exception;
    }

    private void run(PluginCall call, Job job) {
        io.execute(() -> {
            try {
                JSObject out = job.go();
                call.resolve(out != null ? out : new JSObject());
            } catch (Exception e) {
                String msg = e.getMessage();
                call.reject(msg != null && !msg.isEmpty() ? msg : "mail operation failed", e);
            }
        });
    }

    private synchronized AccountStore accounts() throws Exception {
        if (store == null) store = new AccountStore(getContext());
        return store;
    }

    // --- Account CRUD ---

    @PluginMethod
    public void listAccounts(PluginCall call) {
        run(call, () -> {
            JSArray items = new JSArray();
            JSONArray all = accounts().load();
            for (int i = 0; i < all.length(); i++) items.put(publicView(all.getJSONObject(i)));
            return new JSObject().put("accounts", items);
        });
    }

    @PluginMethod
    public void addAccount(PluginCall call) {
        run(call, () -> {
            String email = trim(call.getString("email"));
            String imapHost = trim(call.getString("imapHost"));
            String smtpHost = trim(call.getString("smtpHost"));
            String password = call.getString("password", "");
            int imapPort = intOr(call.getInt("imapPort"), 993);
            int smtpPort = intOr(call.getInt("smtpPort"), 465);
            if (email == null || !email.contains("@") || imapHost == null || smtpHost == null || password.isEmpty()) {
                throw new IllegalArgumentException("email, hosts and password required");
            }
            if (imapPort < 1 || imapPort > 65535 || smtpPort < 1 || smtpPort > 65535) {
                throw new IllegalArgumentException("port out of range");
            }
            // These values land in RFC822 headers — refuse CRLF up front.
            if (hasCtl(email) || hasCtl(imapHost) || hasCtl(smtpHost)) {
                throw new IllegalArgumentException("invalid characters");
            }
            JSONObject a = new JSONObject();
            a.put("id", UUID.randomUUID().toString());
            a.put("name", trim(call.getString("name")) != null ? trim(call.getString("name")) : "");
            a.put("email", email);
            a.put("imapHost", imapHost);
            a.put("imapPort", imapPort);
            a.put("smtpHost", smtpHost);
            a.put("smtpPort", smtpPort);
            String username = trim(call.getString("username"));
            a.put("username", username != null ? username : email);
            a.put("password", password);
            a.put("insecure", call.getBoolean("insecure", false));
            a.put("createdAt", MailUtil.iso(new Date()));
            accounts().add(a);
            return new JSObject().put("account", publicView(a));
        });
    }

    @PluginMethod
    public void removeAccount(PluginCall call) {
        run(call, () -> {
            if (!accounts().remove(call.getString("id", ""))) throw new MessagingException("not found");
            return null;
        });
    }

    @PluginMethod
    public void testAccount(PluginCall call) {
        run(call, () -> {
            // IMAP login + SMTP auth — a send-only failure shouldn't pass.
            JSONObject a = accounts().get(call.getString("id", ""));
            Store s = imapConnect(a);
            s.close();
            Transport t = smtpTransport(a);
            t.close();
            return new JSObject().put("ok", true);
        });
    }

    /**
     * Full records incl. passwords — exists for the server-import path only.
     * Requires an explicit tap on a native dialog, so page script alone can
     * never read stored passwords.
     */
    @PluginMethod
    public void exportAccounts(PluginCall call) {
        String destination = call.getString("destination", "");
        io.execute(() -> {
            int count;
            try {
                count = accounts().load().length();
            } catch (Exception e) {
                call.reject("could not read accounts", e);
                return;
            }
            if (count == 0) {
                call.resolve(new JSObject().put("accounts", new JSArray()));
                return;
            }
            String where = destination == null || destination.isEmpty() ? "your Cal server" : destination;
            String message = "Copy " + count + (count == 1 ? " mail account" : " mail accounts")
                + ", including passwords, to " + where + "?";
            getActivity().runOnUiThread(() -> new AlertDialog.Builder(getActivity())
                .setTitle("Copy mail accounts?")
                .setMessage(message)
                .setNegativeButton("Cancel", (d, w) -> call.reject("cancelled"))
                .setPositiveButton("Copy", (d, w) ->
                    run(call, () -> new JSObject().put("accounts", new JSArray(accounts().load().toString()))))
                .setOnCancelListener(d -> call.reject("cancelled"))
                .show());
        });
    }

    // --- Mailbox + message reads ---

    @PluginMethod
    public void mailboxes(PluginCall call) {
        run(call, () -> {
            Store s = imapConnect(accounts().get(call.getString("id", "")));
            try {
                JSArray boxes = new JSArray();
                String delim = String.valueOf(s.getDefaultFolder().getSeparator());
                for (Folder f : s.getDefaultFolder().list("*")) {
                    boxes.put(new JSObject().put("name", f.getFullName()).put("delimiter", delim));
                }
                return new JSObject().put("mailboxes", boxes);
            } finally {
                s.close();
            }
        });
    }

    @PluginMethod
    public void messages(PluginCall call) {
        run(call, () -> {
            String box = call.getString("mailbox", "INBOX");
            int page = call.getInt("page", 0);
            Store s = imapConnect(accounts().get(call.getString("id", "")));
            try {
                Folder f = s.getFolder(box);
                f.open(Folder.READ_ONLY);
                try {
                    int total = f.getMessageCount();
                    JSObject out = new JSObject().put("total", total);
                    JSArray items = new JSArray();
                    int[] w = MailUtil.windowBounds(total, page);
                    if (w != null) {
                        Message[] msgs = f.getMessages(w[0], w[1]);
                        FetchProfile fp = new FetchProfile();
                        fp.add(FetchProfile.Item.ENVELOPE);
                        fp.add(FetchProfile.Item.FLAGS);
                        fp.add(UIDFolder.FetchProfileItem.UID);
                        fp.add(IMAPFolder.FetchProfileItem.SIZE);
                        f.fetch(msgs, fp);
                        // newest first, like the server implementation
                        for (int i = msgs.length - 1; i >= 0; i--) {
                            Message m = msgs[i];
                            JSObject o = new JSObject();
                            o.put("uid", (int) ((UIDFolder) f).getUID(m));
                            o.put("from", addr(m.getFrom()));
                            o.put("to", addrList(m.getRecipients(Message.RecipientType.TO)));
                            o.put("subject", m.getSubject() != null ? m.getSubject() : "");
                            // jarvis: envelope sent-date; INTERNALDATE isn't in the javax FetchProfile items
                            Date d = m.getSentDate() != null ? m.getSentDate() : m.getReceivedDate();
                            o.put("date", MailUtil.iso(d));
                            o.put("seen", m.isSet(Flags.Flag.SEEN));
                            o.put("size", m.getSize());
                            items.put(o);
                        }
                    }
                    return out.put("messages", items);
                } finally {
                    f.close(false);
                }
            } finally {
                s.close();
            }
        });
    }

    @PluginMethod
    public void message(PluginCall call) {
        run(call, () -> {
            String box = call.getString("mailbox", "INBOX");
            long uid = call.getInt("uid", 0);
            Store s = imapConnect(accounts().get(call.getString("id", "")));
            try {
                Folder f = s.getFolder(box);
                f.open(Folder.READ_ONLY);
                try {
                    Message m = ((UIDFolder) f).getMessageByUID(uid);
                    if (m == null) throw new MessagingException("message not found");
                    String[] body = extractBody(m);
                    return new JSObject()
                        .put("uid", (int) uid)
                        .put("from", addr(m.getFrom()))
                        .put("to", addrList(m.getRecipients(Message.RecipientType.TO)))
                        .put("subject", m.getSubject() != null ? m.getSubject() : "")
                        .put("text", body[0])
                        .put("html", body[1]);
                } finally {
                    f.close(false);
                }
            } finally {
                s.close();
            }
        });
    }

    // --- Flags / delete / send ---

    @PluginMethod
    public void setSeen(PluginCall call) {
        run(call, () -> {
            String box = call.getString("mailbox", "INBOX");
            long uid = call.getInt("uid", 0);
            boolean seen = call.getBoolean("seen", true);
            Store s = imapConnect(accounts().get(call.getString("id", "")));
            try {
                Folder f = s.getFolder(box);
                f.open(Folder.READ_WRITE);
                try {
                    Message m = ((UIDFolder) f).getMessageByUID(uid);
                    if (m == null) throw new MessagingException("message not found");
                    m.setFlag(Flags.Flag.SEEN, seen);
                } finally {
                    f.close(false);
                }
            } finally {
                s.close();
            }
            return null;
        });
    }

    @PluginMethod
    public void deleteMessage(PluginCall call) {
        run(call, () -> {
            String box = call.getString("mailbox", "INBOX");
            long uid = call.getInt("uid", 0);
            Store s = imapConnect(accounts().get(call.getString("id", "")));
            try {
                Folder f = s.getFolder(box);
                f.open(Folder.READ_WRITE);
                try {
                    Message m = ((UIDFolder) f).getMessageByUID(uid);
                    if (m == null) throw new MessagingException("message not found");
                    // Prefer copy→Trash; fall back to \Deleted + expunge.
                    boolean moved = false;
                    try {
                        Folder trash = s.getFolder("Trash");
                        if (!trash.exists()) trash.create(Folder.HOLDS_MESSAGES);
                        f.copyMessages(new Message[]{m}, trash);
                        moved = true;
                    } catch (MessagingException ignored) {
                        // no Trash semantics — expunge it is
                    }
                    m.setFlag(Flags.Flag.DELETED, true);
                    f.close(true); // close(true) expunges \Deleted — MOVE semantics either way
                } finally {
                    if (f.isOpen()) f.close(false);
                }
            } finally {
                s.close();
            }
            return null;
        });
    }

    @PluginMethod
    public void send(PluginCall call) {
        run(call, () -> {
            JSONObject acct = accounts().get(call.getString("id", ""));
            String to = trim(call.getString("to"));
            String cc = trim(call.getString("cc"));
            if (to == null || !to.contains("@") || hasCtl(to) || hasCtl(cc)) {
                throw new IllegalArgumentException("recipient required");
            }

            Session session = Session.getInstance(smtpProps(acct));

            MimeMessage msg = new MimeMessage(session);
            msg.setFrom(new InternetAddress(acct.getString("email")));
            msg.setRecipients(Message.RecipientType.TO, InternetAddress.parse(to));
            if (cc != null) msg.setRecipients(Message.RecipientType.CC, InternetAddress.parse(cc));
            msg.setSubject(MailUtil.cleanHeader(call.getString("subject", "")), "UTF-8");
            msg.setSentDate(new Date());

            String text = call.getString("text", "");
            JSArray atts = call.getArray("attachments", new JSArray());
            if (atts.length() == 0) {
                msg.setText(text, "UTF-8");
            } else {
                MimeMultipart multi = new MimeMultipart();
                MimeBodyPart body = new MimeBodyPart();
                body.setText(text, "UTF-8");
                multi.addBodyPart(body);
                for (int i = 0; i < atts.length() && i < 10; i++) {
                    JSONObject a = atts.getJSONObject(i);
                    byte[] data = Base64.decode(a.optString("data", ""), Base64.DEFAULT);
                    if (data.length == 0) continue;
                    MimeBodyPart part = new MimeBodyPart();
                    part.setDataHandler(new javax.activation.DataHandler(
                        new ByteArrayDataSource(data, a.optString("mime", "application/octet-stream"))));
                    part.setFileName(MailUtil.cleanHeader(a.optString("name", "attachment")));
                    part.setDisposition(Part.ATTACHMENT);
                    multi.addBodyPart(part);
                }
                msg.setContent(multi);
            }
            msg.saveChanges();

            Transport t = smtpTransport(acct);
            try {
                t.sendMessage(msg, msg.getAllRecipients());
            } finally {
                t.close();
            }
            return null;
        });
    }

    // --- SMTP plumbing ---

    /** JavaMail props for the account's SMTP transport. 465 → smtps
     *  (implicit TLS), anything else → smtp + STARTTLS. */
    private static Properties smtpProps(JSONObject acct) throws JSONException {
        boolean ssl = acct.getInt("smtpPort") == 465;
        String proto = ssl ? "smtps" : "smtp";
        Properties props = new Properties();
        props.put("mail." + proto + ".connectiontimeout", "12000");
        props.put("mail." + proto + ".timeout", "15000");
        props.put("mail." + proto + ".writetimeout", "15000");
        props.put("mail." + proto + ".auth", "true");
        if (!ssl) props.put("mail." + proto + ".starttls.enable", "true");
        // Same trust-override flag as IMAP — self-hosted mail, self-signed cert.
        if (acct.optBoolean("insecure")) {
            props.put("mail." + proto + ".ssl.trust", "*");
            props.put("mail." + proto + ".ssl.checkserveridentity", "false");
        }
        return props;
    }

    /** Connected + authenticated SMTP transport for the account. */
    private static Transport smtpTransport(JSONObject acct) throws Exception {
        Properties props = smtpProps(acct);
        Transport t = Session.getInstance(props)
            .getTransport(acct.getInt("smtpPort") == 465 ? "smtps" : "smtp");
        t.connect(acct.getString("smtpHost"), acct.getInt("smtpPort"),
            acct.getString("username"), acct.getString("password"));
        return t;
    }

    // --- IMAP plumbing ---

    private Store imapConnect(JSONObject acct) throws MessagingException {
        boolean ssl = acct.optInt("imapPort", 993) != 143;
        String proto = ssl ? "imaps" : "imap";
        Properties props = new Properties();
        props.put("mail." + proto + ".connectiontimeout", "12000");
        props.put("mail." + proto + ".timeout", "15000");
        props.put("mail." + proto + ".writetimeout", "15000");
        if (!ssl) props.put("mail.imap.starttls.enable", "true");
        // Self-hosted mail often has a self-signed cert — the account flag
        // opts that account into trust-all + no hostname check.
        if (acct.optBoolean("insecure")) {
            props.put("mail." + proto + ".ssl.trust", "*");
            props.put("mail." + proto + ".ssl.checkserveridentity", "false");
        }
        Store s = Session.getInstance(props).getStore(proto);
        s.connect(acct.optString("imapHost"), acct.optInt("imapPort", 993),
            acct.optString("username"), acct.optString("password"));
        return s;
    }

    // --- helpers ---

    private static String trim(String s) {
        if (s == null) return null;
        s = s.trim();
        return s.isEmpty() ? null : s;
    }

    private static int intOr(Integer v, int fallback) {
        return v == null || v == 0 ? fallback : v;
    }

    private static boolean hasCtl(String s) {
        return s != null && (s.indexOf('\r') >= 0 || s.indexOf('\n') >= 0);
    }

    private static String addr(Address[] addrs) {
        if (addrs == null || addrs.length == 0) return "";
        if (addrs[0] instanceof InternetAddress ia) {
            return ia.getPersonal() != null ? ia.getPersonal() + " <" + ia.getAddress() + ">" : ia.getAddress();
        }
        return addrs[0].toString();
    }

    private static JSArray addrList(Address[] addrs) {
        JSArray out = new JSArray();
        if (addrs != null) for (Address a : addrs) out.put(a.toString());
        return out;
    }

    /** Walks the MIME tree; prefers text/plain, keeps text/html as fallback. */
    private static String[] extractBody(Part p) throws Exception {
        String[] acc = {"", ""};
        walk(p, acc);
        return acc;
    }

    private static void walk(Part p, String[] acc) throws Exception {
        if (p.isMimeType("multipart/*")) {
            Multipart mp = (Multipart) p.getContent();
            for (int i = 0; i < mp.getCount(); i++) walk(mp.getBodyPart(i), acc);
        } else if (p.isMimeType("text/plain") && acc[0].isEmpty()) {
            acc[0] = cap(p.getContent().toString());
        } else if (p.isMimeType("text/html") && acc[1].isEmpty()) {
            acc[1] = cap(p.getContent().toString());
        }
    }

    private static String cap(String s) {
        return s.length() > BODY_CAP ? s.substring(0, BODY_CAP) : s;
    }

    private static JSObject publicView(JSONObject a) {
        JSObject o = new JSObject();
        for (String k : new String[]{"id", "name", "email", "imapHost", "imapPort", "smtpHost", "smtpPort", "username", "insecure", "createdAt"}) {
            o.put(k, a.opt(k));
        }
        return o;
    }

    // --- AccountStore: JSON blob, AES-256-GCM via AndroidKeyStore ---

    private static final class AccountStore {
        private static final String PREFS = "cal_mail";
        private static final String KEY = "accounts";
        private static final String KEY_ALIAS = "cal_mail_key";

        private final SharedPreferences prefs;

        AccountStore(Context ctx) {
            prefs = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        }

        synchronized JSONArray load() throws Exception {
            String blob = prefs.getString(KEY, null);
            if (blob == null) return new JSONArray();
            byte[] raw = Base64.decode(blob, Base64.DEFAULT);
            Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
            c.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, raw, 0, 12));
            return new JSONArray(new String(c.doFinal(raw, 12, raw.length - 12), StandardCharsets.UTF_8));
        }

        synchronized JSONObject get(String id) throws Exception {
            JSONArray all = load();
            for (int i = 0; i < all.length(); i++) {
                JSONObject a = all.getJSONObject(i);
                if (a.optString("id").equals(id)) return a;
            }
            throw new MessagingException("unknown account");
        }

        synchronized void add(JSONObject a) throws Exception {
            JSONArray all = load();
            all.put(a);
            save(all);
        }

        synchronized boolean remove(String id) throws Exception {
            JSONArray all = load();
            List<JSONObject> keep = new ArrayList<>();
            boolean found = false;
            for (int i = 0; i < all.length(); i++) {
                JSONObject a = all.getJSONObject(i);
                if (a.optString("id").equals(id)) found = true;
                else keep.add(a);
            }
            if (found) save(new JSONArray(keep));
            return found;
        }

        private void save(JSONArray all) throws Exception {
            byte[] plain = all.toString().getBytes(StandardCharsets.UTF_8);
            Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
            byte[] iv = new byte[12];
            new SecureRandom().nextBytes(iv);
            c.init(Cipher.ENCRYPT_MODE, key(), new GCMParameterSpec(128, iv));
            byte[] enc = c.doFinal(plain);
            byte[] blob = new byte[12 + enc.length];
            System.arraycopy(iv, 0, blob, 0, 12);
            System.arraycopy(enc, 0, blob, 12, enc.length);
            prefs.edit().putString(KEY, Base64.encodeToString(blob, Base64.DEFAULT)).apply();
        }

        private static SecretKey key() throws Exception {
            KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
            ks.load(null);
            if (!ks.containsAlias(KEY_ALIAS)) {
                KeyGenerator kg = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
                kg.init(new KeyGenParameterSpec.Builder(KEY_ALIAS,
                        KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setKeySize(256)
                    .build());
                kg.generateKey();
            }
            return ((KeyStore.SecretKeyEntry) ks.getEntry(KEY_ALIAS, null)).getSecretKey();
        }
    }
}

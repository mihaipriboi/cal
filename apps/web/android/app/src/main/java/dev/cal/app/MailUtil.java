package dev.cal.app;

import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.Locale;
import java.util.Properties;
import java.util.TimeZone;

/** Pure helpers for CalMailPlugin — kept class-free so JVM tests need no Android. */
final class MailUtil {

    static final int PAGE_SIZE = 40;

    private MailUtil() {}

    // minSdk 23 — java.time/Date.toInstant need API 26; SimpleDateFormat is safe.
    static String iso(Date d) {
        if (d == null) return "";
        SimpleDateFormat f = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US);
        f.setTimeZone(TimeZone.getTimeZone("UTC"));
        return f.format(d);
    }

    /** Same newest-first seq window as the Go mail handler. Null = empty page. */
    static int[] windowBounds(int total, int page) {
        if (total == 0) return null;
        int end = total - page * PAGE_SIZE;
        int start = Math.max(1, end - PAGE_SIZE + 1);
        return end < 1 ? null : new int[]{start, end};
    }

    /** Strip CR/LF so a header value can't inject extra headers. */
    static String cleanHeader(String s) {
        return s == null ? "" : s.replace("\r", "").replace("\n", "");
    }

    /** TLS settings shared by IMAP and SMTP. Plain ports must upgrade with
     *  STARTTLS (no silent cleartext fallback), and the certificate must match
     *  the host: JavaMail 1.6 skips that check unless asked. The per-account
     *  "insecure" flag (self-hosted mail, self-signed cert) opts that account
     *  into trust-all + no hostname check, as before. */
    static void tlsProps(Properties props, String proto, boolean implicitTls, boolean insecure) {
        if (!implicitTls) {
            props.put("mail." + proto + ".starttls.enable", "true");
            props.put("mail." + proto + ".starttls.required", "true");
        }
        if (insecure) {
            props.put("mail." + proto + ".ssl.trust", "*");
            props.put("mail." + proto + ".ssl.checkserveridentity", "false");
        } else {
            props.put("mail." + proto + ".ssl.checkserveridentity", "true");
        }
    }
}

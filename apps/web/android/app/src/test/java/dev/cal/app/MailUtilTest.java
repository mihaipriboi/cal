package dev.cal.app;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

import java.util.Date;
import java.util.Properties;

public class MailUtilTest {

    @Test
    public void windowBounds_newestPage() {
        assertArrayEquals(new int[]{161, 200}, MailUtil.windowBounds(200, 0));
    }

    @Test
    public void windowBounds_olderPagesAndClamp() {
        assertArrayEquals(new int[]{121, 160}, MailUtil.windowBounds(200, 1));
        assertArrayEquals(new int[]{1, 37}, MailUtil.windowBounds(37, 0));
        assertArrayEquals(new int[]{1, 3}, MailUtil.windowBounds(123, 3));
    }

    @Test
    public void windowBounds_empty() {
        assertNull(MailUtil.windowBounds(0, 0));
        assertNull(MailUtil.windowBounds(200, 6));
    }

    @Test
    public void cleanHeader_stripsInjection() {
        assertEquals("hiBcc: evil@x", MailUtil.cleanHeader("hi\r\nBcc: evil@x"));
        assertEquals("", MailUtil.cleanHeader(null));
    }

    @Test
    public void iso_utcRfc3339() {
        assertEquals("2026-09-12T08:30:00Z", MailUtil.iso(new Date(1789201800000L)));
        assertEquals("", MailUtil.iso(null));
    }

    @Test
    public void tlsProps_plainPortRequiresStarttlsAndHostCheck() {
        Properties p = new Properties();
        MailUtil.tlsProps(p, "imap", false, false);
        assertEquals("true", p.getProperty("mail.imap.starttls.enable"));
        assertEquals("true", p.getProperty("mail.imap.starttls.required"));
        assertEquals("true", p.getProperty("mail.imap.ssl.checkserveridentity"));
        assertNull(p.getProperty("mail.imap.ssl.trust"));
    }

    @Test
    public void tlsProps_implicitTlsChecksHost() {
        Properties p = new Properties();
        MailUtil.tlsProps(p, "smtps", true, false);
        assertNull(p.getProperty("mail.smtps.starttls.enable"));
        assertEquals("true", p.getProperty("mail.smtps.ssl.checkserveridentity"));
    }

    @Test
    public void tlsProps_insecureOptOutKeepsStarttlsRequired() {
        Properties p = new Properties();
        MailUtil.tlsProps(p, "smtp", false, true);
        assertEquals("true", p.getProperty("mail.smtp.starttls.required"));
        assertEquals("*", p.getProperty("mail.smtp.ssl.trust"));
        assertEquals("false", p.getProperty("mail.smtp.ssl.checkserveridentity"));
    }
}

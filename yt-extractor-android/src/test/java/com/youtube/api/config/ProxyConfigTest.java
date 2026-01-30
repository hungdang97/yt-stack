package com.youtube.api.config;

import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

public class ProxyConfigTest {

    @Test
    public void testParseProxyWithSpecialCharInPassword() {
        // Test với proxy từ screenshot: rivernet_proxyer:rivernet123@:23.88.64.58:21589
        String proxyString = "rivernet_proxyer:rivernet123@:23.88.64.58:21589";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("rivernet_proxyer", config.getUsername());
        assertEquals("rivernet123@", config.getPassword());
        assertEquals("23.88.64.58", config.getIp());
        assertEquals(21589, config.getPort());
        assertTrue(config.hasAuth());
    }

    @Test
    public void testParseProxySimple() {
        String proxyString = "user:pass:192.168.1.1:8080";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("user", config.getUsername());
        assertEquals("pass", config.getPassword());
        assertEquals("192.168.1.1", config.getIp());
        assertEquals(8080, config.getPort());
    }

    @Test
    public void testParseProxyNoAuth() {
        String proxyString = "::192.168.1.1:8080";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("", config.getUsername());
        assertEquals("", config.getPassword());
        assertEquals("192.168.1.1", config.getIp());
        assertEquals(8080, config.getPort());
        assertFalse(config.hasAuth());
    }

    @Test
    public void testParseProxyNull() {
        ProxyConfig config = ProxyConfig.parse(null);
        assertNull(config);
    }

    @Test
    public void testParseProxyEmpty() {
        ProxyConfig config = ProxyConfig.parse("");
        assertNull(config);
    }

    @Test
    public void testParseProxyInvalidFormat() {
        assertThrows(IllegalArgumentException.class, () -> {
            ProxyConfig.parse("invalid");
        });
    }

    @Test
    public void testParseProxyInvalidPort() {
        assertThrows(IllegalArgumentException.class, () -> {
            ProxyConfig.parse("user:pass:192.168.1.1:invalid");
        });
    }

    // ========== NEW URL FORMAT TESTS ==========

    @Test
    public void testParseUrlFormat() {
        // Same format as crawler_extract/premium_cookie_extract
        String proxyString = "http://spwjp5q0ka:g_qgYfrQn4bO4k42Hg@dc.decodo.com:10001";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("spwjp5q0ka", config.getUsername());
        assertEquals("g_qgYfrQn4bO4k42Hg", config.getPassword());
        assertEquals("dc.decodo.com", config.getIp());
        assertEquals(10001, config.getPort());
        assertTrue(config.hasAuth());
    }

    @Test
    public void testParseUrlFormatNoAuth() {
        String proxyString = "http://proxy.example.com:8080";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("", config.getUsername());
        assertEquals("", config.getPassword());
        assertEquals("proxy.example.com", config.getIp());
        assertEquals(8080, config.getPort());
        assertFalse(config.hasAuth());
    }

    @Test
    public void testParseUrlFormatWithUrlEncodedPassword() {
        // Password contains @ encoded as %40
        String proxyString = "http://user:pass%40word@proxy.com:3128";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("user", config.getUsername());
        assertEquals("pass@word", config.getPassword()); // Decoded
        assertEquals("proxy.com", config.getIp());
        assertEquals(3128, config.getPort());
    }

    @Test
    public void testParseUrlFormatWithoutProtocol() {
        // Without http:// prefix
        String proxyString = "user:password@192.168.1.1:8080";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("user", config.getUsername());
        assertEquals("password", config.getPassword());
        assertEquals("192.168.1.1", config.getIp());
        assertEquals(8080, config.getPort());
    }

    @Test
    public void testParseUrlFormatHttps() {
        String proxyString = "https://admin:secret@secure-proxy.io:443";
        ProxyConfig config = ProxyConfig.parse(proxyString);

        assertNotNull(config);
        assertEquals("admin", config.getUsername());
        assertEquals("secret", config.getPassword());
        assertEquals("secure-proxy.io", config.getIp());
        assertEquals(443, config.getPort());
    }
}

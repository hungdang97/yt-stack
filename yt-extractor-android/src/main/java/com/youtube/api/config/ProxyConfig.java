package com.youtube.api.config;

import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@NoArgsConstructor
@AllArgsConstructor
public class ProxyConfig {
    private String username;
    private String password;
    private String ip;
    private int port;

    /**
     * Parse proxy string format:
     * - URL format: http://username:password@host:port (same as crawler_extract/premium)
     * - URL format (no auth): http://host:port
     * - Legacy format: username:password:ip:port
     * - Simple format: ip:port
     * @param proxyString proxy in various formats
     * @return ProxyConfig object or null if invalid
     */
    public static ProxyConfig parse(String proxyString) {
        if (proxyString == null || proxyString.trim().isEmpty()) {
            return null;
        }

        try {
            String proxy = proxyString.trim();

            // Strip http:// or https:// prefix
            if (proxy.startsWith("http://")) {
                proxy = proxy.substring(7);
            } else if (proxy.startsWith("https://")) {
                proxy = proxy.substring(8);
            }

            // Check for URL format: username:password@host:port (with @ symbol)
            // Use LAST @ to handle passwords containing @
            // Also check that what follows @ is a valid host (not starting with :)
            int atIndex = proxy.lastIndexOf('@');
            if (atIndex != -1) {
                String afterAt = proxy.substring(atIndex + 1);
                // Valid URL format: after @ should be host:port, not :port
                // If it starts with : it's likely legacy format with @ in password
                if (!afterAt.isEmpty() && !afterAt.startsWith(":")) {
                    return parseUrlFormat(proxy, atIndex);
                }
            }

            // Legacy/simple format parsing
            return parseLegacyFormat(proxy);
        } catch (NumberFormatException e) {
            throw new IllegalArgumentException("Invalid port number in proxy string");
        } catch (IllegalArgumentException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalArgumentException("Invalid proxy format. Expected: http://user:pass@host:port or user:pass:ip:port");
        }
    }

    /**
     * Parse URL format: username:password@host:port
     * Supports URL-encoded credentials (e.g., special chars like @ encoded as %40)
     */
    private static ProxyConfig parseUrlFormat(String proxy, int atIndex) {
        String credentials = proxy.substring(0, atIndex);
        String hostPort = proxy.substring(atIndex + 1);

        // Parse host:port
        int lastColon = hostPort.lastIndexOf(':');
        if (lastColon == -1) {
            throw new IllegalArgumentException("Invalid proxy format: missing port");
        }

        String host = hostPort.substring(0, lastColon).trim();
        String portStr = hostPort.substring(lastColon + 1).trim();
        int port = Integer.parseInt(portStr);

        if (host.isEmpty() || port <= 0 || port > 65535) {
            throw new IllegalArgumentException("Invalid host or port");
        }

        // Parse username:password
        String username = "";
        String password = "";

        if (!credentials.isEmpty()) {
            int colonIndex = credentials.indexOf(':');
            if (colonIndex != -1) {
                username = urlDecode(credentials.substring(0, colonIndex));
                password = urlDecode(credentials.substring(colonIndex + 1));
            } else {
                username = urlDecode(credentials);
            }
        }

        return new ProxyConfig(username, password, host, port);
    }

    /**
     * Parse legacy format: username:pass:ip:port OR ip:port
     */
    private static ProxyConfig parseLegacyFormat(String proxy) {
        // Find the last colon for port
        int lastColon = proxy.lastIndexOf(':');
        if (lastColon == -1) {
            throw new IllegalArgumentException("Invalid proxy format");
        }

        String portStr = proxy.substring(lastColon + 1).trim();
        int port = Integer.parseInt(portStr);

        String remaining = proxy.substring(0, lastColon);
        int secondLastColon = remaining.lastIndexOf(':');

        // Simple format: ip:port (no auth)
        if (secondLastColon == -1) {
            String ip = remaining.trim();
            if (ip.isEmpty() || port <= 0 || port > 65535) {
                throw new IllegalArgumentException("Invalid IP or port");
            }
            return new ProxyConfig("", "", ip, port);
        }

        // Full format: username:pass:ip:port
        String ip = remaining.substring(secondLastColon + 1).trim();
        String credentials = remaining.substring(0, secondLastColon);
        int firstColon = credentials.indexOf(':');

        String username = "";
        String password = "";

        if (firstColon != -1) {
            username = credentials.substring(0, firstColon).trim();
            password = credentials.substring(firstColon + 1).trim();
        } else {
            username = credentials.trim();
        }

        if (ip.isEmpty() || port <= 0 || port > 65535) {
            throw new IllegalArgumentException("Invalid IP or port");
        }

        return new ProxyConfig(username, password, ip, port);
    }

    /**
     * Decode URL-encoded string (e.g., %40 -> @, %3A -> :)
     */
    private static String urlDecode(String str) {
        if (str == null || str.isEmpty()) {
            return str;
        }
        try {
            return java.net.URLDecoder.decode(str, "UTF-8");
        } catch (Exception e) {
            return str; // Return original if decoding fails
        }
    }

    public boolean hasAuth() {
        return username != null && !username.isEmpty() && password != null && !password.isEmpty();
    }
}

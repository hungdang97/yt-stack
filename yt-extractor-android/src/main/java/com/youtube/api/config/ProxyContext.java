package com.youtube.api.config;

/**
 * ThreadLocal context to store proxy configuration for current request
 */
public class ProxyContext {
    private static final ThreadLocal<ProxyConfig> proxyHolder = new ThreadLocal<>();

    public static void setProxy(ProxyConfig proxy) {
        proxyHolder.set(proxy);
    }

    public static ProxyConfig getProxy() {
        return proxyHolder.get();
    }

    public static void clear() {
        proxyHolder.remove();
    }

    public static boolean hasProxy() {
        return proxyHolder.get() != null;
    }
}

package com.youtube.api.config;

import okhttp3.*;
import org.schabi.newpipe.extractor.downloader.Downloader;
import org.schabi.newpipe.extractor.downloader.Request;
import org.schabi.newpipe.extractor.downloader.Response;
import org.schabi.newpipe.extractor.exceptions.ReCaptchaException;
import org.springframework.stereotype.Component;

import javax.annotation.Nonnull;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.Proxy;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;

@Component
public class DownloaderImpl extends Downloader {

    public static final String USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";

    private final OkHttpClient defaultClient;

    public DownloaderImpl() {
        this.defaultClient = createClient(null);
    }

    private OkHttpClient createClient(ProxyConfig proxyConfig) {
        OkHttpClient.Builder builder = new OkHttpClient.Builder()
                .readTimeout(30, TimeUnit.SECONDS)
                .connectTimeout(30, TimeUnit.SECONDS)
                .followRedirects(true)
                .followSslRedirects(true)
                .addInterceptor(chain -> {
                    okhttp3.Request original = chain.request();
                    okhttp3.Request request = original.newBuilder()
                            .header("User-Agent", USER_AGENT)
                            .build();
                    return chain.proceed(request);
                });

        if (proxyConfig != null) {
            Proxy proxy = new Proxy(Proxy.Type.HTTP,
                    new InetSocketAddress(proxyConfig.getIp(), proxyConfig.getPort()));
            builder.proxy(proxy);

            if (proxyConfig.hasAuth()) {
                builder.proxyAuthenticator((route, response) -> {
                    String credential = Credentials.basic(proxyConfig.getUsername(), proxyConfig.getPassword());
                    return response.request().newBuilder()
                            .header("Proxy-Authorization", credential)
                            .build();
                });
            }
        }

        return builder.build();
    }

    private OkHttpClient getClient() {
        ProxyConfig proxyConfig = ProxyContext.getProxy();
        if (proxyConfig != null) {
            return createClient(proxyConfig);
        }
        return defaultClient;
    }

    @Override
    public Response execute(@Nonnull Request request) throws IOException, ReCaptchaException {
        final String httpMethod = request.httpMethod();
        final String url = request.url();
        final Map<String, List<String>> headers = request.headers();
        final byte[] dataToSend = request.dataToSend();

        RequestBody requestBody = null;
        if (dataToSend != null) {
            requestBody = RequestBody.create(dataToSend);
        }

        final okhttp3.Request.Builder requestBuilder = new okhttp3.Request.Builder()
                .method(httpMethod, requestBody)
                .url(url);

        if (headers != null) {
            for (Map.Entry<String, List<String>> pair : headers.entrySet()) {
                final String headerName = pair.getKey();
                final List<String> headerValueList = pair.getValue();

                if (headerValueList.size() > 1) {
                    requestBuilder.removeHeader(headerName);
                    for (String headerValue : headerValueList) {
                        requestBuilder.addHeader(headerName, headerValue);
                    }
                } else if (headerValueList.size() == 1) {
                    requestBuilder.header(headerName, headerValueList.get(0));
                }
            }
        }

        okhttp3.Response response = null;

        try {
            response = getClient().newCall(requestBuilder.build()).execute();

            if (response.code() == 429) {
                response.close();
                throw new ReCaptchaException("reCaptcha Challenge requested", url);
            }

            final ResponseBody body = response.body();
            String responseBodyToReturn = null;

            if (body != null) {
                responseBodyToReturn = body.string();
            }

            final String latestUrl = response.request().url().toString();
            return new Response(response.code(), response.message(), response.headers().toMultimap(),
                    responseBodyToReturn, latestUrl);
        } catch (IOException e) {
            if (e.getMessage() != null && e.getMessage().contains("127.0.0.1:8888")) {
                ProxyConfig proxyConfig = ProxyContext.getProxy();
                if (proxyConfig != null) {
                    throw new IOException("Proxy connection failed: " + proxyConfig.getIp() + ":"
                            + proxyConfig.getPort() + ". Make sure proxy server is running.", e);
                }
            }
            throw e;
        } finally {
            if (response != null) {
                response.close();
            }
        }
    }
}

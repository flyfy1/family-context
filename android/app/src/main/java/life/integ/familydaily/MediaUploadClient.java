package life.integ.familydaily;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.UUID;

final class MediaUploadClient {
    interface ConnectionFactory {
        HttpURLConnection open(URL url) throws IOException;
    }

    private final ConnectionFactory connections;

    MediaUploadClient() {
        this(url -> (HttpURLConnection) url.openConnection());
    }

    MediaUploadClient(ConnectionFactory connections) {
        this.connections = connections;
    }

    void upload(String baseUrl, String memberToken, InputStream media, String fileName, String mimeType,
                String capturedAt, String deviceId, String clientMediaId) throws IOException {
        String boundary = "FamilyDailyMedia-" + UUID.randomUUID();
        URL url = new URL(trimTrailingSlash(baseUrl) + "/api/v1/me/media-imports");
        HttpURLConnection connection = connections.open(url);
        connection.setRequestMethod("POST");
        connection.setConnectTimeout(15_000);
        connection.setReadTimeout(180_000);
        connection.setDoOutput(true);
        connection.setChunkedStreamingMode(64 * 1024);
        connection.setRequestProperty("Accept", "application/json");
        connection.setRequestProperty("Authorization", "Bearer " + memberToken);
        connection.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);
        try (OutputStream output = new BufferedOutputStream(connection.getOutputStream());
             InputStream input = new BufferedInputStream(media)) {
            field(output, boundary, "capturedAt", capturedAt);
            field(output, boundary, "deviceId", deviceId);
            field(output, boundary, "clientMediaId", clientMediaId);
            write(output, "--" + boundary + "\r\n");
            write(output, "Content-Disposition: form-data; name=\"media\"; filename=\"" + safeFileName(fileName) + "\"\r\n");
            write(output, "Content-Type: " + mimeType + "\r\n\r\n");
            byte[] buffer = new byte[64 * 1024];
            int read;
            while ((read = input.read(buffer)) != -1) {
                output.write(buffer, 0, read);
            }
            write(output, "\r\n--" + boundary + "--\r\n");
        }
        int status = connection.getResponseCode();
        if (status < 200 || status >= 300) {
            String message = readError(connection);
            throw new HttpUploadException(status, message.isEmpty() ? "上传失败（" + status + "）" : message);
        }
        try (InputStream response = connection.getInputStream()) {
            while (response.read() != -1) {
                // Drain the response so the connection can be reused.
            }
        }
    }

    static String trimTrailingSlash(String value) {
        String result = value == null ? "" : value.trim();
        while (result.endsWith("/")) result = result.substring(0, result.length() - 1);
        return result;
    }

    static boolean isValidBaseUrl(String value) {
        try {
            URL url = new URL(value);
            return ("https".equals(url.getProtocol()) || "http".equals(url.getProtocol()))
                    && url.getHost() != null && !url.getHost().isEmpty()
                    && (url.getPath().isEmpty() || "/".equals(url.getPath()));
        } catch (Exception ignored) {
            return false;
        }
    }

    private static void field(OutputStream output, String boundary, String name, String value) throws IOException {
        if (value == null || value.isEmpty()) return;
        write(output, "--" + boundary + "\r\nContent-Disposition: form-data; name=\"" + name + "\"\r\n\r\n" + value + "\r\n");
    }

    private static String safeFileName(String value) {
        return value.replace("\\", "_").replace("/", "_").replace("\"", "_").replace("\r", "_").replace("\n", "_");
    }

    private static void write(OutputStream output, String value) throws IOException {
        output.write(value.getBytes(StandardCharsets.UTF_8));
    }

    private static String readError(HttpURLConnection connection) throws IOException {
        InputStream stream = connection.getErrorStream();
        if (stream == null) return "";
        try (InputStream input = stream; ByteArrayOutputStream output = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[4096];
            int read;
            while ((read = input.read(buffer)) != -1 && output.size() < 32 * 1024) output.write(buffer, 0, read);
            return output.toString(StandardCharsets.UTF_8.name());
        }
    }

    static final class HttpUploadException extends IOException {
        final int statusCode;

        HttpUploadException(int statusCode, String message) {
            super(message);
            this.statusCode = statusCode;
        }
    }
}

package life.integ.familydaily;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.io.ByteArrayOutputStream;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;

public class MediaUploadClientTest {
    @Test
    public void sendsMemberAuthAndStableIdToExistingEndpoint() throws Exception {
        FakeConnection connection = new FakeConnection(new URL("https://nas.example/api/v1/me/media-imports"));
        new MediaUploadClient(url -> connection).upload("https://nas.example/", "member-secret",
                new ByteArrayInputStream("jpeg-data".getBytes(StandardCharsets.UTF_8)), "camera.jpg", "image/jpeg",
                "2026-08-22T05:00:00Z", "device-1", "mediastore-image-42");
        String body = connection.output.toString(StandardCharsets.UTF_8.name());
        assertEquals("Bearer member-secret", connection.authorization);
        assertEquals("/api/v1/me/media-imports", connection.getURL().getPath());
        assertTrue(body.contains("name=\"clientMediaId\"\r\n\r\nmediastore-image-42"));
        assertTrue(body.contains("name=\"deviceId\"\r\n\r\ndevice-1"));
        assertTrue(body.contains("name=\"media\"; filename=\"camera.jpg\""));
        assertTrue(body.contains("jpeg-data"));
    }

    @Test
    public void normalizesTrailingSlash() {
        assertEquals("https://nas.example", MediaUploadClient.trimTrailingSlash(" https://nas.example/// "));
        assertTrue(MediaUploadClient.isValidBaseUrl("https://nas.example"));
        assertTrue(MediaUploadClient.isValidBaseUrl("http://192.168.1.8:8080"));
    }

    private static final class FakeConnection extends HttpURLConnection {
        final ByteArrayOutputStream output = new ByteArrayOutputStream();
        String authorization;

        FakeConnection(URL url) {
            super(url);
        }

        @Override public void setRequestProperty(String key, String value) {
            if ("Authorization".equals(key)) authorization = value;
            super.setRequestProperty(key, value);
        }

        @Override public OutputStream getOutputStream() {
            return output;
        }

        @Override public int getResponseCode() {
            return 201;
        }

        @Override public InputStream getInputStream() {
            return new ByteArrayInputStream("{}".getBytes(StandardCharsets.UTF_8));
        }

        @Override public void disconnect() {}
        @Override public boolean usingProxy() { return false; }
        @Override public void connect() throws IOException {}
    }
}

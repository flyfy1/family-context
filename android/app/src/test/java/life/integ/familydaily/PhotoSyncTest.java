package life.integ.familydaily;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

public class PhotoSyncTest {
    private static final String PRODUCTION = "https://family-api.integ.life";

    @Test
    public void migratesStaleEmulatorAndLoopbackAddressesToProduction() {
        assertEquals(PRODUCTION, PhotoSync.resolveBaseUrl("http://127.0.0.1:18083", PRODUCTION));
        assertEquals(PRODUCTION, PhotoSync.resolveBaseUrl("http://localhost:8080", PRODUCTION));
        assertEquals(PRODUCTION, PhotoSync.resolveBaseUrl("http://10.0.2.2:8080", PRODUCTION));
    }

    @Test
    public void preservesRealCustomServiceAndLocalDevelopmentTargets() {
        assertEquals("https://family-nas.example", PhotoSync.resolveBaseUrl("https://family-nas.example/", PRODUCTION));
        assertEquals("http://127.0.0.1:18083", PhotoSync.resolveBaseUrl("http://127.0.0.1:18083", "http://10.0.2.2:8080"));
        assertEquals(PRODUCTION, PhotoSync.resolveBaseUrl("", PRODUCTION));
    }
}

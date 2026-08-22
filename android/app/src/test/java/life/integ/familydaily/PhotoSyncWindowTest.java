package life.integ.familydaily;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class PhotoSyncWindowTest {
    private static final long DAY = 24L * 60L * 60L * 1000L;

    @Test
    public void defaultsToThreeDaysAndIncludesBoundary() {
        long now = 10L * DAY;
        long cutoff = PhotoSyncWindow.cutoffMillis(now, PhotoSyncWindow.DEFAULT_DAYS);
        assertTrue(PhotoSyncWindow.includes(now - 2 * DAY, 0, cutoff));
        assertTrue(PhotoSyncWindow.includes(now - 3 * DAY, 0, cutoff));
        assertFalse(PhotoSyncWindow.includes(now - 3 * DAY - 1, 0, cutoff));
    }

    @Test
    public void fallsBackToMediaStoreAddedTimeWhenCaptureTimeMissing() {
        long now = 10L * DAY;
        long cutoff = PhotoSyncWindow.cutoffMillis(now, 3);
        assertTrue(PhotoSyncWindow.includes(0, (now - DAY) / 1000L, cutoff));
        assertFalse(PhotoSyncWindow.includes(0, (now - 4 * DAY) / 1000L, cutoff));
    }

    @Test
    public void validatesConfigurableRange() {
        assertTrue(PhotoSyncWindow.isValidDays(1));
        assertTrue(PhotoSyncWindow.isValidDays(3650));
        assertFalse(PhotoSyncWindow.isValidDays(0));
        assertFalse(PhotoSyncWindow.isValidDays(3651));
    }
}

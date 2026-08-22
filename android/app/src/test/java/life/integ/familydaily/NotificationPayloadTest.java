package life.integ.familydaily;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class NotificationPayloadTest {
    @Test
    public void parsesActionableHTTPSNotification() throws Exception {
        NotificationPayload payload = NotificationPayload.fromFields("notice-1", "家里想你了",
                "点这里看看家人的近况。", "https://family.integ.life/#/feed");
        assertEquals("notice-1", payload.id);
        assertEquals("家里想你了", payload.title);
        assertEquals("https://family.integ.life/#/feed", payload.actionUrl);
    }

    @Test
    public void acceptsOnlyHTTPSWebActions() {
        assertTrue(NotificationPayload.isSafeActionURL("https://family.integ.life/#/feed"));
        assertFalse(NotificationPayload.isSafeActionURL("http://family.integ.life"));
        assertFalse(NotificationPayload.isSafeActionURL("intent://settings"));
        assertFalse(NotificationPayload.isSafeActionURL("https://user:secret@family.integ.life"));
    }

    @Test
    public void rejectsNonActionableInboxRows() throws Exception {
        boolean rejected = false;
        try {
            NotificationPayload.fromFields("old-notice", "", "旧提醒", "");
        } catch (org.json.JSONException expected) {
            rejected = true;
        }
        assertTrue("missing action URL should be rejected", rejected);
    }
}

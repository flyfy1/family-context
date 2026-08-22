package life.integ.familydaily;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

public class MemberProfileSettingsTest {
    @Test
    public void supportsThreeModesAndFallsBackToMember() {
        assertEquals("member", MemberProfileSettings.normalizeRole(null));
        assertEquals("member", MemberProfileSettings.normalizeRole("admin"));
        assertEquals("member", MemberProfileSettings.normalizeRole("member"));
        assertEquals("elder", MemberProfileSettings.normalizeRole("elder"));
        assertEquals("child", MemberProfileSettings.normalizeRole("child"));
    }
}

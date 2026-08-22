package life.integ.familydaily;

import android.content.Context;
import android.content.SharedPreferences;

final class MemberProfileSettings {
    static final String MEMBER = "member";
    static final String ELDER = "elder";
    static final String CHILD = "child";
    private static final String PREFS = "family_daily_profile";
    private static final String KEY_ID = "member_id";
    private static final String KEY_NAME = "member_name";
    private static final String KEY_ROLE = "member_role";

    private MemberProfileSettings() {}

    static Profile get(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        return new Profile(prefs.getString(KEY_ID, ""), prefs.getString(KEY_NAME, ""), normalizeRole(prefs.getString(KEY_ROLE, MEMBER)));
    }

    static void set(Context context, String id, String name, String role) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putString(KEY_ID, id == null ? "" : id)
                .putString(KEY_NAME, name == null ? "" : name)
                .putString(KEY_ROLE, normalizeRole(role))
                .apply();
    }

    static String normalizeRole(String role) {
        if (ELDER.equals(role)) return ELDER;
        if (CHILD.equals(role)) return CHILD;
        return MEMBER;
    }

    static final class Profile {
        final String id;
        final String name;
        final String role;

        Profile(String id, String name, String role) {
            this.id = id;
            this.name = name;
            this.role = role;
        }
    }
}

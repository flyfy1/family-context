package life.integ.familydaily;

import android.content.Context;
import android.content.SharedPreferences;

final class MemberSessionSettings {
    private static final String PREFS = "family_daily_session";
    private static final String KEY_ACCESS_TOKEN = "access_token";
    private static final String KEY_EXPIRES_AT = "expires_at";
    private static final String KEY_MEMBER_ID = "member_id";
    private static final String KEY_MEMBER_NAME = "member_name";
    private static final String KEY_MEMBER_ROLE = "member_role";

    private MemberSessionSettings() {}

    static Session get(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        return new Session(
                prefs.getString(KEY_ACCESS_TOKEN, ""),
                prefs.getString(KEY_EXPIRES_AT, ""),
                prefs.getString(KEY_MEMBER_ID, ""),
                prefs.getString(KEY_MEMBER_NAME, ""),
                MemberProfileSettings.normalizeRole(prefs.getString(KEY_MEMBER_ROLE, MemberProfileSettings.MEMBER))
        );
    }

    static void save(Context context, String accessToken, String expiresAt, String memberId, String memberName, String memberRole) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putString(KEY_ACCESS_TOKEN, accessToken == null ? "" : accessToken)
                .putString(KEY_EXPIRES_AT, expiresAt == null ? "" : expiresAt)
                .putString(KEY_MEMBER_ID, memberId == null ? "" : memberId)
                .putString(KEY_MEMBER_NAME, memberName == null ? "" : memberName)
                .putString(KEY_MEMBER_ROLE, MemberProfileSettings.normalizeRole(memberRole))
                .apply();
    }

    static void clear(Context context) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().clear().apply();
    }

    static final class Session {
        final String accessToken;
        final String expiresAt;
        final String memberId;
        final String memberName;
        final String memberRole;

        Session(String accessToken, String expiresAt, String memberId, String memberName, String memberRole) {
            this.accessToken = accessToken == null ? "" : accessToken;
            this.expiresAt = expiresAt == null ? "" : expiresAt;
            this.memberId = memberId == null ? "" : memberId;
            this.memberName = memberName == null ? "" : memberName;
            this.memberRole = MemberProfileSettings.normalizeRole(memberRole);
        }

        boolean isAuthenticated() {
            return !accessToken.isEmpty() && !memberId.isEmpty();
        }

        MemberProfileSettings.Profile profile() {
            return new MemberProfileSettings.Profile(memberId, memberName, memberRole);
        }
    }
}

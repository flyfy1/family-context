package life.integ.familydaily;

import android.content.Context;
import android.content.SharedPreferences;

final class LanguageSettings {
    static final String ENGLISH = "en";
    static final String CHINESE = "zh";
    private static final String PREFS = "family_daily_language";
    private static final String KEY_LANGUAGE = "language";

    private LanguageSettings() {}

    static String get(Context context) {
        return normalize(context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .getString(KEY_LANGUAGE, ENGLISH));
    }

    static void set(Context context, String language) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putString(KEY_LANGUAGE, normalize(language)).apply();
    }

    static String normalize(String language) {
        return CHINESE.equalsIgnoreCase(language == null ? "" : language.trim()) ? CHINESE : ENGLISH;
    }

    static String text(String language, String english, String chinese) {
        return CHINESE.equals(normalize(language)) ? chinese : english;
    }
}

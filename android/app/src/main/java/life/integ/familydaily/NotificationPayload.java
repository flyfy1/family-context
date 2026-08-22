package life.integ.familydaily;

import org.json.JSONException;
import org.json.JSONObject;

import java.net.URI;

final class NotificationPayload {
    final String id;
    final String title;
    final String message;
    final String actionUrl;

    NotificationPayload(String id, String title, String message, String actionUrl) {
        this.id = id;
        this.title = title;
        this.message = message;
        this.actionUrl = actionUrl;
    }

    static NotificationPayload fromJSON(JSONObject value, boolean preferEnglish) throws JSONException {
        return fromLocalizedFields(value.getString("id"), value.optString("title", "Family Daily"),
                value.optString("titleEn", ""), value.getString("message"), value.optString("messageEn", ""),
                value.optString("actionUrl", ""), preferEnglish);
    }

    static NotificationPayload fromLocalizedFields(String id, String title, String titleEn, String message,
                                                    String messageEn, String actionUrl, boolean preferEnglish) throws JSONException {
        if (preferEnglish) {
            title = localizedOrFallback(titleEn, title);
            message = localizedOrFallback(messageEn, message);
        }
        return fromFields(id, title, message, actionUrl);
    }

    private static String localizedOrFallback(String localized, String fallback) {
        return localized == null || localized.trim().isEmpty() ? fallback : localized;
    }

    static NotificationPayload fromFields(String id, String title, String message, String actionUrl) throws JSONException {
        id = id == null ? "" : id.trim();
        title = title == null ? "" : title.trim();
        message = message == null ? "" : message.trim();
        actionUrl = actionUrl == null ? "" : actionUrl.trim();
        if (id.isEmpty() || message.isEmpty() || !isSafeActionURL(actionUrl)) {
            throw new JSONException("invalid notification payload");
        }
        if (title.isEmpty()) title = "Family Daily";
        return new NotificationPayload(id, title, message, actionUrl);
    }

    static boolean isSafeActionURL(String value) {
        try {
            URI uri = URI.create(value);
            return "https".equalsIgnoreCase(uri.getScheme()) && uri.getHost() != null && uri.getUserInfo() == null;
        } catch (IllegalArgumentException error) {
            return false;
        }
    }
}

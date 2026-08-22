package life.integ.familydaily;

import android.Manifest;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;

import androidx.annotation.NonNull;
import androidx.work.Worker;
import androidx.work.WorkerParameters;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.BufferedInputStream;
import java.io.ByteArrayOutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashSet;
import java.util.Collections;
import java.util.Locale;
import java.util.Set;

public final class NotificationSyncWorker extends Worker {
    private static final String CHANNEL_ID = "family_updates";
    private static final String PREFS = "delivered_notifications";
    private static final String KEY_IDS = "ids";

    public NotificationSyncWorker(@NonNull Context context, @NonNull WorkerParameters parameters) {
        super(context, parameters);
    }

    @NonNull
    @Override
    public Result doWork() {
        Context context = getApplicationContext();
        MemberSessionSettings.Session session = MemberSessionSettings.get(context);
        if (!session.isAuthenticated() || !canNotify(context)) return Result.success();
        try {
            HttpURLConnection connection = (HttpURLConnection) new URL(BuildConfig.API_BASE_URL + "/api/v1/me/notifications").openConnection();
            connection.setRequestMethod("GET");
            connection.setConnectTimeout(10_000);
            connection.setReadTimeout(15_000);
            connection.setRequestProperty("Authorization", "Bearer " + session.accessToken);
            int status = connection.getResponseCode();
            if (status == 401 || status == 403) return Result.failure();
            if (status < 200 || status >= 300) return Result.retry();
            JSONObject response = new JSONObject(readAll(connection));
            JSONArray notifications = response.optJSONArray("notifications");
            if (notifications == null) return Result.success();
            SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
            Set<String> delivered = new HashSet<>(prefs.getStringSet(KEY_IDS, Collections.emptySet()));
            for (int index = notifications.length() - 1; index >= 0; index--) {
                JSONObject value = notifications.getJSONObject(index);
                if (!value.isNull("readAt") || delivered.contains(value.optString("id"))) continue;
                NotificationPayload payload;
                try {
                    payload = NotificationPayload.fromJSON(value, "en".equalsIgnoreCase(Locale.getDefault().getLanguage()));
                } catch (JSONException invalidOrNonActionable) {
                    continue;
                }
                show(context, payload);
                delivered.add(payload.id);
            }
            prefs.edit().putStringSet(KEY_IDS, delivered).apply();
            return Result.success();
        } catch (Exception error) {
            return Result.retry();
        }
    }

    private static boolean canNotify(Context context) {
        return Build.VERSION.SDK_INT < 33 || context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED;
    }

    private static void show(Context context, NotificationPayload payload) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel(CHANNEL_ID, "Family reminders", NotificationManager.IMPORTANCE_HIGH));
        Intent open = new Intent(Intent.ACTION_VIEW, Uri.parse(payload.actionUrl));
        PendingIntent pending = PendingIntent.getActivity(context, payload.id.hashCode(), open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification notification = new Notification.Builder(context, CHANNEL_ID)
                .setSmallIcon(android.R.drawable.ic_dialog_info)
                .setContentTitle(payload.title)
                .setContentText(payload.message)
                .setStyle(new Notification.BigTextStyle().bigText(payload.message))
                .setContentIntent(pending)
                .setAutoCancel(true)
                .build();
        manager.notify(payload.id.hashCode(), notification);
    }

    private static String readAll(HttpURLConnection connection) throws Exception {
        try (BufferedInputStream input = new BufferedInputStream(connection.getInputStream());
             ByteArrayOutputStream output = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[4096];
            int read;
            while ((read = input.read(buffer)) >= 0) output.write(buffer, 0, read);
            return output.toString(StandardCharsets.UTF_8.name());
        }
    }
}

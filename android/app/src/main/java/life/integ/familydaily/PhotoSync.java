package life.integ.familydaily;

import android.Manifest;
import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.os.Build;

import androidx.work.Constraints;
import androidx.work.ExistingPeriodicWorkPolicy;
import androidx.work.NetworkType;
import androidx.work.OneTimeWorkRequest;
import androidx.work.PeriodicWorkRequest;
import androidx.work.WorkManager;

import java.util.UUID;
import java.util.concurrent.TimeUnit;

final class PhotoSync {
    static final String PREFS = "photo-sync";
    static final String KEY_ENABLED = "enabled";
    static final String KEY_BASE_URL = "base-url";
    static final String KEY_LOOKBACK_DAYS = "lookback-days";
    static final String KEY_DEVICE_ID = "device-id";
    static final String KEY_CURSOR_SECONDS = "cursor-seconds";
    static final String KEY_CURSOR_ID = "cursor-id";
    static final String KEY_STATUS = "status";
    static final String KEY_LAST_SUCCESS_MS = "last-success-ms";
    static final String PERIODIC_WORK = "family-daily-photo-sync";
    static final String IMMEDIATE_WORK = "family-daily-photo-sync-now";

    private PhotoSync() {}

    static SharedPreferences preferences(Context context) {
        return context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    static String deviceId(Context context) {
        SharedPreferences prefs = preferences(context);
        String value = prefs.getString(KEY_DEVICE_ID, "");
        if (value != null && !value.isEmpty()) return value;
        value = UUID.randomUUID().toString();
        prefs.edit().putString(KEY_DEVICE_ID, value).apply();
        return value;
    }

    static boolean isConfigured(Context context) {
        SharedPreferences prefs = preferences(context);
        return !prefs.getString(KEY_BASE_URL, "").trim().isEmpty()
                && MemberSessionSettings.get(context).isAuthenticated();
    }

    static int lookbackDays(Context context) {
        return preferences(context).getInt(KEY_LOOKBACK_DAYS, PhotoSyncWindow.DEFAULT_DAYS);
    }

    static boolean hasImageAccess(Context context) {
        if (Build.VERSION.SDK_INT >= 33) {
            return context.checkSelfPermission(Manifest.permission.READ_MEDIA_IMAGES) == PackageManager.PERMISSION_GRANTED;
        }
        return context.checkSelfPermission(Manifest.permission.READ_EXTERNAL_STORAGE) == PackageManager.PERMISSION_GRANTED;
    }

    static String[] imagePermissions() {
        if (Build.VERSION.SDK_INT >= 34) {
            return new String[]{Manifest.permission.READ_MEDIA_IMAGES, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED};
        }
        if (Build.VERSION.SDK_INT >= 33) return new String[]{Manifest.permission.READ_MEDIA_IMAGES};
        return new String[]{Manifest.permission.READ_EXTERNAL_STORAGE};
    }

    static void schedule(Context context) {
        Constraints constraints = new Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build();
        PeriodicWorkRequest periodic = new PeriodicWorkRequest.Builder(PhotoSyncWorker.class, 15, TimeUnit.MINUTES)
                .setConstraints(constraints)
                .build();
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(PERIODIC_WORK, ExistingPeriodicWorkPolicy.UPDATE, periodic);
    }

    static void syncNow(Context context) {
        Constraints constraints = new Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build();
        OneTimeWorkRequest request = new OneTimeWorkRequest.Builder(PhotoSyncWorker.class).setConstraints(constraints).build();
        WorkManager.getInstance(context).enqueueUniqueWork(IMMEDIATE_WORK, androidx.work.ExistingWorkPolicy.REPLACE, request);
    }
}

package life.integ.familydaily;

import android.content.ContentResolver;
import android.content.Context;
import android.content.SharedPreferences;
import android.database.Cursor;
import android.net.Uri;
import android.provider.MediaStore;

import androidx.annotation.NonNull;
import androidx.work.Worker;
import androidx.work.WorkerParameters;

import java.io.InputStream;
import java.time.Instant;

public final class PhotoSyncWorker extends Worker {
    private static final long MAX_UPLOAD_BYTES = 100L * 1024L * 1024L;
    private static final int MAX_ITEMS_PER_RUN = 50;

    public PhotoSyncWorker(@NonNull Context context, @NonNull WorkerParameters parameters) {
        super(context, parameters);
    }

    @NonNull
    @Override
    public Result doWork() {
        Context context = getApplicationContext();
        String language = LanguageSettings.get(context);
        SharedPreferences prefs = PhotoSync.preferences(context);
        if (!prefs.getBoolean(PhotoSync.KEY_ENABLED, false) || !PhotoSync.isConfigured(context)) return Result.success();
        if (!PhotoSync.hasImageAccess(context)) {
            setStatus(prefs, tr(language, "Photo access is required to continue syncing", "需要允许照片访问，才能继续同步"));
            return Result.success();
        }

        String baseUrl = prefs.getString(PhotoSync.KEY_BASE_URL, "").trim();
        String memberToken = prefs.getString(PhotoSync.KEY_MEMBER_TOKEN, "").trim();
        int lookbackDays = PhotoSync.lookbackDays(context);
        if (!PhotoSyncWindow.isValidDays(lookbackDays)) {
            setStatus(prefs, tr(language, "Sync paused: the photo lookback range is invalid", "同步暂停：照片回溯天数设置不正确"));
            return Result.failure();
        }
        long cutoffMillis = PhotoSyncWindow.cutoffMillis(System.currentTimeMillis(), lookbackDays);
        long cursorSeconds = prefs.getLong(PhotoSync.KEY_CURSOR_SECONDS, 0);
        long cursorId = prefs.getLong(PhotoSync.KEY_CURSOR_ID, 0);
        int uploaded = 0;
        int skipped = 0;
        boolean more = false;
        ContentResolver resolver = context.getContentResolver();
        Uri collection = MediaStore.Images.Media.EXTERNAL_CONTENT_URI;
        String[] projection = {
                MediaStore.Images.Media._ID,
                MediaStore.Images.Media.DISPLAY_NAME,
                MediaStore.Images.Media.MIME_TYPE,
                MediaStore.Images.Media.DATE_TAKEN,
                MediaStore.Images.Media.DATE_ADDED,
                MediaStore.Images.Media.SIZE
        };
        String capturedInWindow = "((" + MediaStore.Images.Media.DATE_TAKEN + " IS NOT NULL AND " + MediaStore.Images.Media.DATE_TAKEN + " > 0 AND "
                + MediaStore.Images.Media.DATE_TAKEN + " >= ?) OR ((" + MediaStore.Images.Media.DATE_TAKEN + " IS NULL OR "
                + MediaStore.Images.Media.DATE_TAKEN + " = 0) AND " + MediaStore.Images.Media.DATE_ADDED + " >= ?))";
        String afterCursor = "(" + MediaStore.Images.Media.DATE_ADDED + " > ? OR (" + MediaStore.Images.Media.DATE_ADDED + " = ? AND "
                + MediaStore.Images.Media._ID + " > ?))";
        String selection = capturedInWindow + " AND " + afterCursor;
        String cutoffSeconds = Long.toString(cutoffMillis / 1000L);
        String[] args = {Long.toString(cutoffMillis), cutoffSeconds, Long.toString(cursorSeconds), Long.toString(cursorSeconds), Long.toString(cursorId)};
        String order = MediaStore.Images.Media.DATE_ADDED + " ASC, " + MediaStore.Images.Media._ID + " ASC";

        try (Cursor media = resolver.query(collection, projection, selection, args, order)) {
            if (media == null) throw new IllegalStateException(tr(language, "Unable to read the photo library", "无法读取系统相册"));
            int idColumn = media.getColumnIndexOrThrow(MediaStore.Images.Media._ID);
            int nameColumn = media.getColumnIndexOrThrow(MediaStore.Images.Media.DISPLAY_NAME);
            int mimeColumn = media.getColumnIndexOrThrow(MediaStore.Images.Media.MIME_TYPE);
            int takenColumn = media.getColumnIndexOrThrow(MediaStore.Images.Media.DATE_TAKEN);
            int addedColumn = media.getColumnIndexOrThrow(MediaStore.Images.Media.DATE_ADDED);
            int sizeColumn = media.getColumnIndexOrThrow(MediaStore.Images.Media.SIZE);
            MediaUploadClient client = new MediaUploadClient();
            while (media.moveToNext()) {
                if (uploaded + skipped >= MAX_ITEMS_PER_RUN) {
                    more = true;
                    break;
                }
                long id = media.getLong(idColumn);
                long added = media.getLong(addedColumn);
                String mime = media.getString(mimeColumn);
                long size = media.getLong(sizeColumn);
                if (!supportedMime(mime) || size <= 0 || size > MAX_UPLOAD_BYTES) {
                    skipped++;
                    saveCursor(prefs, added, id);
                    continue;
                }
                Uri uri = Uri.withAppendedPath(collection, Long.toString(id));
                long taken = media.isNull(takenColumn) ? added * 1000L : media.getLong(takenColumn);
                String name = media.getString(nameColumn);
                if (name == null || name.trim().isEmpty()) name = "photo-" + id;
                setStatus(prefs, tr(language, "Syncing photo " + (uploaded + 1) + "…", "正在同步第 " + (uploaded + 1) + " 张照片……"));
                try (InputStream input = resolver.openInputStream(uri)) {
                    if (input == null) throw new IllegalStateException(tr(language, "Unable to open the photo", "无法打开照片"));
                    client.upload(baseUrl, memberToken, input, name, mime, Instant.ofEpochMilli(taken).toString(),
                            PhotoSync.deviceId(context), "mediastore-image-" + id);
                }
                uploaded++;
                saveCursor(prefs, added, id);
            }
        } catch (SecurityException denied) {
            setStatus(prefs, tr(language, "Photo permission changed; allow access again", "照片权限已变化，请重新允许访问"));
            return Result.success();
        } catch (MediaUploadClient.HttpUploadException httpError) {
            if (httpError.statusCode == 400 || httpError.statusCode == 401 || httpError.statusCode == 403) {
                setStatus(prefs, tr(language, "Sync paused: check the NAS address, member token, or photo format", "同步暂停：请检查 NAS 地址、成员令牌或照片格式"));
                return Result.failure();
            }
            setStatus(prefs, tr(language, "The network or service is unavailable; sync will retry later", "网络或服务暂时不可用，稍后自动重试"));
            return Result.retry();
        } catch (Exception error) {
            setStatus(prefs, tr(language, "Sync was interrupted and will retry later", "同步暂时中断，稍后自动重试"));
            return Result.retry();
        }

        String summary = uploaded == 0
                ? tr(language, "Photos from the last " + lookbackDays + " days are up to date", "最近 " + lookbackDays + " 天的照片已是最新")
                : tr(language, "Synced " + uploaded + " photos from the last " + lookbackDays + " days to the private NAS Space", "已同步最近 " + lookbackDays + " 天内的 " + uploaded + " 张照片到私人 NAS 空间");
        if (skipped > 0) summary += tr(language, "; skipped " + skipped + " unsupported or oversized files", "，跳过 " + skipped + " 个不支持或过大的文件");
        prefs.edit().putString(PhotoSync.KEY_STATUS, summary).putLong(PhotoSync.KEY_LAST_SUCCESS_MS, System.currentTimeMillis()).apply();
        return more ? Result.retry() : Result.success();
    }

    private static boolean supportedMime(String mime) {
        return "image/jpeg".equals(mime) || "image/png".equals(mime) || "image/webp".equals(mime) || "image/gif".equals(mime);
    }

    private static void saveCursor(SharedPreferences prefs, long seconds, long id) {
        prefs.edit().putLong(PhotoSync.KEY_CURSOR_SECONDS, seconds).putLong(PhotoSync.KEY_CURSOR_ID, id).apply();
    }

    private static void setStatus(SharedPreferences prefs, String value) {
        prefs.edit().putString(PhotoSync.KEY_STATUS, value).apply();
    }

    private static String tr(String language, String english, String chinese) {
        return LanguageSettings.text(language, english, chinese);
    }
}

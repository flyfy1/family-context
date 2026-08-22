package life.integ.familydaily;

final class PhotoSyncWindow {
    static final int DEFAULT_DAYS = 3;
    static final int MIN_DAYS = 1;
    static final int MAX_DAYS = 3650;
    private static final long DAY_MILLIS = 24L * 60L * 60L * 1000L;

    private PhotoSyncWindow() {}

    static boolean isValidDays(int days) {
        return days >= MIN_DAYS && days <= MAX_DAYS;
    }

    static long cutoffMillis(long nowMillis, int days) {
        if (!isValidDays(days)) throw new IllegalArgumentException("lookback days out of range");
        return nowMillis - days * DAY_MILLIS;
    }

    static boolean includes(long capturedAtMillis, long addedAtSeconds, long cutoffMillis) {
        long effectiveTime = capturedAtMillis > 0 ? capturedAtMillis : addedAtSeconds * 1000L;
        return effectiveTime >= cutoffMillis;
    }
}

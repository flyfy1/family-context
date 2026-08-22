package life.integ.familydaily;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

public final class NotificationDemoReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (MemberSessionSettings.get(context).isAuthenticated()) {
            NotificationSync.syncNow(context);
        }
    }
}

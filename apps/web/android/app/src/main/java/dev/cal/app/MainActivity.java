package dev.cal.app;

import android.content.Intent;
import android.os.Bundle;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {

    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(WidgetConfigPlugin.class);
        registerPlugin(CalMailPlugin.class);
        super.onCreate(savedInstanceState);
        routeShare(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        routeShare(intent);
    }

    // ACTION_SEND text → deep-link the web app to /share so the editor opens
    // prefilled. Runs inside the WebView via loadUrl javascript? No — the PWA
    // share_target handles browsers; here we reload the activity with a
    // query-string deep link.
    private void routeShare(Intent intent) {
        if (intent == null || !Intent.ACTION_SEND.equals(intent.getAction())) return;
        String text = intent.getStringExtra(Intent.EXTRA_TEXT);
        if (text == null || text.isEmpty()) return;
        String url = "share?text=" + android.net.Uri.encode(text)
            + (text.matches(".*https?://\\S+.*")
                ? "&url=" + android.net.Uri.encode(text.replaceAll(".*?(https?://\\S+).*", "$1"))
                : "");
        bridge.getWebView().post(() -> bridge.getWebView().loadUrl(bridge.getLocalUrl() + "/" + url));
    }
}

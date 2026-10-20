import type { CapacitorConfig } from "@capacitor/cli";

// The Android shell wraps the built PWA. The API URL is runtime-relative so
// the same bundle works against any self-hosted origin; set server.url during
// development to point at a live dev server if needed.
const config: CapacitorConfig = {
  appId: "dev.cal.app",
  appName: "Cal",
  webDir: "dist",
  server: {
    // https origin; the server must be reachable over HTTPS (cleartext is
    // blocked by res/xml/network_security_config.xml).
    androidScheme: "https",
  },
  android: {
    allowMixedContent: false,
    // Android 15+ enforces edge-to-edge; without this the app draws under
    // the status bar (WebView doesn't populate env(safe-area-inset-*)).
    adjustMarginsForEdgeToEdge: "auto",
  },
  plugins: {
    SplashScreen: { launchAutoHide: true },
  },
};

export default config;

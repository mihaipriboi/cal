import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig, loadEnv, type Plugin } from "vite";
import { VitePWA } from "vite-plugin-pwa";

// Content-Security-Policy for the built app shell, as a <meta> tag so it also
// applies where no server sets headers (the Android/iOS WebView). It mirrors
// the server's policy except connect-src, which needs https: because native
// builds call a user-chosen server origin. When the server also sends its
// header (web), browsers enforce both, so the web build stays as strict as
// before. Build-only: the Vite dev server needs inline scripts for HMR.
const appCsp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: https:",
  "font-src 'self' data:",
  "connect-src 'self' https:",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
].join("; ");

const cspMeta: Plugin = {
  name: "cal-csp-meta",
  apply: "build",
  transformIndexHtml: (html) =>
    html.replace("<head>", `<head>
    <meta http-equiv="Content-Security-Policy" content="${appCsp}" />`),
};

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".");
  return {
    // Playwright specs live in e2e/ — vitest runs only src.
    test: { exclude: ["e2e/**", "node_modules/**", "android/**"] },
    plugins: [
      cspMeta,
      react(),
      tailwindcss(),
      VitePWA({
        registerType: "autoUpdate",
        includeAssets: ["favicon.svg"],
        manifest: {
          name: "Cal",
          short_name: "Cal",
          description: "Self-hosted minimal planner",
          theme_color: "#151513",
          background_color: "#151513",
          display: "standalone",
          start_url: "/",
          share_target: {
            action: "/share",
            method: "GET",
            enctype: "application/x-www-form-urlencoded",
            params: { title: "title", text: "text", url: "url" }
          },
          icons: [
            { src: "/pwa-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
            { src: "/pwa-512.png", sizes: "512x512", type: "image/png", purpose: "any maskable" },
            { src: "/pwa-512.svg", sizes: "any", type: "image/svg+xml", purpose: "any" }
          ]
        },
        workbox: {
          importScripts: ["/sw-push.js"],
          runtimeCaching: [
            {
              urlPattern: ({ url }) => url.pathname.startsWith("/api/entries") || url.pathname.startsWith("/api/holidays"),
              handler: "NetworkFirst",
              options: { cacheName: "cal-api" }
            }
          ]
        }
      })
    ],
    server: {
      proxy: {
        "/api": env.VITE_API_ORIGIN ?? "http://localhost:8082"
      }
    }
  };
});

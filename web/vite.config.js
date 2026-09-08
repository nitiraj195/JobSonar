import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// A browser page-navigation and an in-app fetch() can hit the identical
// path -- GET /jobs/<uuid> is both the React Router detail route and the
// API call JobDetail.jsx makes to load it. Proxy rules match by path
// alone, so without this they're indistinguishable: a direct navigation
// or refresh on /jobs/<uuid> would be proxied straight to the Go API
// (raw JSON) instead of reaching the SPA shell. bypass() tells Vite's
// dev proxy to serve index.html instead of proxying whenever the
// request's Accept header says "this is a page load", which only a
// real browser navigation sets, not fetch().
function bypassNavigations(req) {
  // Binary downloads are real navigations too (Accept includes text/html).
  // Always proxy those so the <a href> download buttons reach the API.
  if (req.url && /\.docx(\?|$)/i.test(req.url)) {
    return;
  }
  if (req.headers.accept && req.headers.accept.includes("text/html")) {
    return "/index.html";
  }
}

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/jobs": { target: "http://127.0.0.1:8080", bypass: bypassNavigations },
      "/reviews": { target: "http://127.0.0.1:8080", bypass: bypassNavigations },
      "/tailor": { target: "http://127.0.0.1:8080", bypass: bypassNavigations },
      "/applications": { target: "http://127.0.0.1:8080", bypass: bypassNavigations },
      "/profile": { target: "http://127.0.0.1:8080", bypass: bypassNavigations },
      "/companies": { target: "http://127.0.0.1:8080", bypass: bypassNavigations },
    },
  },
});

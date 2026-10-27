# Fetch and render

Reads JSON from one site through WRM and shows it as a table, only while the window is visible.

1. Put your site into `http.origins` in `addon.json` and into `ORIGIN` in `panel.js` (exact `https://host[:port]`, no path).
2. Narrow `http.paths` to what the panel needs (`/api/*` or exact paths).
3. If the site needs a token, keep `http.secrets: ["api_token"]` (sent as `Authorization: Bearer`). Use `basic:api_token` for `user:password` or `header:X-Api-Key:api_token` for a named header. Enter the value in WRM: *Settings → Addons → Instances → 🔑 Secrets*. Remove the entry if no token is needed.
4. `wrm -addon-lint .`, then `wrm -addon-dev .` (the dev server sends the requests itself; give the token as `WRM_ADDON_SECRET_API_TOKEN`).
5. Zip and install. An administrator approves the site before the first request.

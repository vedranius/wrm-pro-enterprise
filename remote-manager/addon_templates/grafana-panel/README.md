# Grafana overview

Shows the health and the firing alerts of one Grafana while the window is open. WRM sends the requests; the token stays in WRM. 24/7 alerting stays in Grafana itself.

1. Replace `https://grafana.example.com` in `addon.json` (`http.origins`) and in `panel.js` (`GRAFANA`) with your Grafana's address.
2. In Grafana create a service account with the *Viewer* role and a token.
3. `wrm -addon-lint .` and try it with `WRM_ADDON_SECRET_GRAFANA_TOKEN=<token> wrm -addon-dev .`
4. Zip and install. An administrator approves the site (tick *private network* if Grafana has a private address), then enter the token in *Settings → Addons → Instances → 🔑 Secrets*.

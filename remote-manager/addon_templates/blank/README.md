# Blank panel

A window that keeps a note on the WRM server.

1. Change `id` in `addon.json` (lowercase, for example `yourname.notes`) and `name`.
2. Check it: `wrm -addon-lint .`
3. Try it with the real sandbox: `wrm -addon-dev .` and open the printed address.
4. Zip the folder and install it in *Settings → Addons*.

`wrm-bridge.js` wraps the messages to WRM (`wrm.ready()`, `wrm.storeGet`, `wrm.storeSet`, `wrm.notify`, `wrm.http`). See docs/ADDONS.md in the WRM repository.

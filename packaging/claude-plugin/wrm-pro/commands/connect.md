---
description: Connect to WRM PRO (sign in in the browser), sign in again, or switch to another WRM
argument-hint: "[wrm-address]"
---
Connect this computer to WRM PRO.

1. If `$1` was given, use it as the WRM address. Otherwise ask the user for the address they open
   WRM with in the browser, for example `http://192.168.1.10:8780` or `https://wrm.example.com`.
2. Call the `wrm_connect` tool with that `address`. WRM opens in the browser, where the user signs
   in and approves which servers, mode and permissions this app gets. The token is stored on this
   computer only (`~/.wrm/mcp.json`); it never passes through the chat.
3. If the result says the sign-in is still open, ask the user to finish it in the browser (give
   the link from the result if no window opened), then call `wrm_connect` again.
4. When connected, call `list_connections` and tell the user which servers they can reach.

Never ask the user to paste a token into the chat. If `wrm_connect` reports that WRM does not
offer sign-in for AI apps, an administrator has to turn on AI connections with OAuth sign-in in
WRM → Settings → AI connections; the alternative is WRM's setup script for all AI apps
(WRM → Settings → AI connections → All AI apps on this computer).

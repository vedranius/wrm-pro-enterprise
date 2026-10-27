# WebAssembly hook

Server code for an organisation addon. WRM runs it with wazero and no WASI: no files, no network, no environment, no clock. Only an administrator can install it, never as a personal addon.

- `addon.json` declares `server.module` and the functions WRM may call (`server.exports`).
- `plugin.wasm` is a placeholder made by WRM that returns `{"ok":true}`. Build your own from `main.go` with TinyGo (`tinygo build -o plugin.wasm -target=wasm-unknown -no-debug .`) or any language that can export `memory`, `wrm_alloc(size) -> ptr` and `status(ptr, len) -> ptr`.
- `wrm -addon-lint .` compiles the module and checks its imports and exports.
- After installing, an administrator calls it with `POST /api/addons/example.hook/call/status`.

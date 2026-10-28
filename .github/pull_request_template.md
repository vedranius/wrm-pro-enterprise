## What does this PR change?

<!-- Short description; link the related issue, e.g. "Fixes #12" -->

## How was it tested?

- [ ] `gofmt -l .` is empty, `go vet ./...` and `go test -race ./...` pass
- [ ] Server-side permission checks for any new endpoint / action; no secrets sent to the browser or logged
- [ ] Tested in the browser (terminal / file manager / sessions as relevant)
- [ ] New UI text has `en` and `hr` translations

## Risk & security

<!-- For changes touching auth, crypto, sessions, the MCP layer, networking, tunnels, addons or
     update, describe the security impact. Such changes require a security review, per
     docs/rfcs/README.md. If none, write "None". -->

## Contributor terms & AI disclosure

- [ ] I have read [CONTRIBUTING.md](../CONTRIBUTING.md) and agree to the contributor terms: my
      contribution is licensed under the [Apache License 2.0](../LICENSE), I have signed the
      [CLA](../CLA.md), and this commit carries a `Signed-off-by` line.
- [ ] If this PR is substantially AI-assisted, I state the tool(s) and model(s) used and what I
      personally verified. See [docs/AI-CONTRIBUTIONS.md](../docs/AI-CONTRIBUTIONS.md).

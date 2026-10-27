# WRM PRO v12.4.3b — Medium Windows runtime hotfix

The owner reported missing rsync while importing/starting Medium in a Windows `.exe` installation and prioritized this fix before v12.4.4. Implementation: [22be697](https://github.com/vedranius/web-browser-RDM-public/commit/22be697). [Release](https://github.com/vedranius/web-browser-RDM-public/releases/tag/v12.4.3b).

## Delivered

- Windows selects a managed Linux Docker backend. Docker Desktop CLI discovery includes standard install locations; an override supports custom locations. Docker must be running with a Linux engine and accessible to the WRM account. Python/Git/OpenSSH/rsync run inside a pinned, previously published WRM Linux image, with the original private app and lockfile imported separately.
- Linux retains native execution when dependencies exist and can select the managed backend explicitly. Browser clients never require rsync/Python. Docker Desktop itself is a one-time prerequisite, not silently installed by WRM. Windows Server needs a supported Linux WRM host/VM; this local bridge is not a remote-Docker integration.
- Source archives remain available after setup failure. Administrator Prepare runtime retries the approved source; failed/pending setup is not ready. Config validation uses the selected runtime.
- Persistent Linux runtime/user volumes preserve real POSIX permissions, symlinks, Git state, original history and backups. Imported config/SSH revisions are applied only on explicit import; ordinary restarts preserve native edits and data. Source/venv are mounted read-only, without the host Docker socket or other operators' user volumes.
- Container environment aliases preserve the Docker CLI's host HOME/context while forwarding runtime values without secret argv. The original app gets the expected environment. OpenSSH `~/.ssh` maps to the user's private Linux profile. Native builds drop to the pinned image's unprivileged `wrm` account, with private config/key read checks.
- Owned container cleanup and authenticated readiness/busy checks remain in WRM. The image's unrelated WRM healthcheck is disabled for the original-tool process. Active native operations cannot be stopped by the normal stop action.
- Letter hotfix parsing/version identity/file selection supports v12.4.3b after v12.4.3 and before v12.4.4. Existing v12.4.3 updater cannot recognize it: manually download/replace the executable for this first letter hotfix.

## Validation

- Final full Go race suite: **533.803 seconds**, passed. Focused runtime selection/argument/missing-Docker/readiness and hotfix-version regressions passed. Vet/gofmt/diff checks passed.
- **4 Python wrapper tests** and **23 Node regressions** passed; UI en/hr syntax/translation check passed.
- Actual provided original tool ZIP prepared through the default pinned Linux image in Chromium, with the managed backend forced. The runtime used its own Linux tools, not host deployment executables.
- Actual OpenSSH/rsync fixture: comparison, an unprivileged build, two deployments with verified file contents, real v2-to-v1 rollback and preserved history after stop/start passed. An initial fixture failure correctly reported a non-writable remote backup parent; granting the test account the intended fixture directory permission resolved it. Standard `~/.ssh` keys resolve inside the private Linux home.
- The build user could not read the private SSH key/config. Active overview requests correctly caused normal Stop to refuse interruption. Browser checks at **390/1100/1440/1920 px** passed without JavaScript errors; screenshots remain local.
- Windows amd64, macOS arm64, FreeBSD amd64 and Linux ARMv7 cross-builds passed. Runtime tests are Linux Docker tests of the same backend; **no native Windows VM execution is claimed**. The Windows Desktop install/context/permission path still needs confirmation on the owner's machine.

## Operating requirements

Retain DB/key/private `.medium` imports/preferences and back up the Linux runtime/user volumes. Do not remove the named user volumes to upgrade: they hold native edits/history. Docker access is privileged; grant Medium only to trusted operators. Additional build dependencies/users can require a trusted custom image; native host SSH agents are a native-backend option, while managed Docker uses the imported profile. Git/SSH destinations must be reachable from the container. The separate original engine retains its own plans/history; no supplied private code/config is published in Git or release assets.

## Next version

[ROADMAP.md](../ROADMAP.md) consolidates all deferred requests and the new import/MCP/Monitoring/log/terminal work into **v12.4.4**. It is accepted planned work, not delivered by this hotfix. Issue #5's final XML/PuTTY report needs exact UI verification before it is declared resolved. Releases still require exact-head green CI, merge, tag, all 16 packages/checksums and Docker publication.

# Medium original-tool parity — v12.4.3

## Reference and preservation

The owner supplied deploy-tool (3) and a matching deploy-config ZIP. Tool ZIP SHA-256:

`43fb33e758cd1f5e6773d1f8d82bdbdbad3cfcb6fcb53dd7f2d123d5d1e3baad`

WRM imports the archive's `app/`, `scripts/`, pinned `requirements.lock` and project metadata into a private runtime. Python modules, original templates/static files and engine files are retained unchanged. The original ASGI application is loaded through an authentication/readiness/busy wrapper. WRM translates root-relative URLs and response headers into `/medium/`; it does not redesign original forms or substitute deployment algorithms.

The supplied config was parsed with the original loader: **5 projects, 7 servers, 6 environments**. Real names and archive contents are intentionally private. Operations were exercised with a separate generic local Git/SSH fixture, not private production destinations.

## Preserved native interface and behavior

| Original feature/routes | Medium behavior | Verification |
|---|---|---|
| Overview `/`, `/pregled/celija` | Original project/environment cells and native state | Original suite and browser |
| Comparison `/usporedba`, environment/destination/branch selectors | Original selectors, allowed refs, server/path targeting and comparison | Original suite and browser comparison |
| Comparison details, `/diff`, `/plan`, `/ignore` | Original per-file status/diffs, selection/exclusions and plan | Original suite; browser plan and deployment |
| Native `/deploy` and job `/log` | Original confirmations, dry run, build/deploy commands, progress/error output, server locking and backups | Original suite; browser deployed two refs and checked actual remote contents |
| `/lock` | Native lock behavior and recovery UI | Original suite; route preserved |
| `/povijest`, `/povijest/rollback` | Native history, per-server state and rollback, including dry run | Original suite; browser dry rollback and actual v2-to-v1 restoration |
| `/postavke/server`, `/okolina`, `/projekt` | Native server/environment/project CRUD, order and all original project editor options | Original suite; browser settings/project editor |
| `/postavke/yaml` validation/save | Native YAML editor/validation | Original suite; route preserved |
| `/postavke/git/{commit,povuci,provjeri}` | Native Git configuration editor actions | Original suite; browser local config-repo clone, native commit/push and fetch/pull through the final WRM binary against a local bare repository |
| Original data layout | Native `current.json`, `history.jsonl`, comparisons, backups and locks under the user's private data directory | Original suite and deploy/rollback browser workflow |

The original source suite completed with **1,581 passed, 9 skipped**. These are the reference's own tests, not a claim that every original action was manually tested in the bridge. WRM regressions separately cover URL rewriting, redirects, authentication/header isolation, same-origin/no-referrer forms, CSRF rejection and wrapper busy state. Chromium exercised original forms without JavaScript errors.

## WRM integration settings

- Original environment preferences: `DEPLOY_CONFIG_REPO`, `DEPLOY_CONFIG_PULL`, `DEPLOY_CONFIG_CHECK_MINUTES`, `DEPLOY_USER_EMAIL`, `DEPLOY_HOSTNAME`, `DEPLOY_SSH_OPTIONS`, `DEPLOY_TIMEOUT`, `DEPLOY_BUILD_USER`, `GIT_SSH_COMMAND`.
- WRM supplies the managed bind/port, private config/data/home directories, authenticated user identity and private bridge key. Credential/command preferences are encrypted and masked on retrieval.
- Flat imported config and optional native Git clone use separate directories. SSH profile upload supports config, keys and known_hosts. Git commits/pushes need the original tool's identity, keys, permissions and network access.
- Executable source import is administrator-only. Default `git_medium=admins`; each native action requires WRM authentication and check/action policies. Native deployments require update/install permissions; native rollback requires rollback permission. The WRM audit records action boundaries, while original history records native outcomes.
- Closing the workspace leaves the original process running. Explicit stop checks active requests/background jobs. WRM shutdown stops processes; persisted native history/config remains.

## Runtime requirements and trust

Python 3.12+, git, OpenSSH and rsync are required; Docker includes them. Builds additionally need their original toolchains and OS user privileges. Private ZIPs are imported after installation and are not release assets. Separate operator directories/processes are not an OS sandbox: trusted native code and editable commands execute as the WRM OS account. Simple/Advanced retain WRM's existing catalog and deploy engine; their history and Medium history are separate.

## Additional final-binary verification

After tagging, Chromium also completed native **Commit i push**, verified the commit on the local bare config origin, then completed **Provjeri sada / Povuci s gita** after a separate fixture commit and verified equal local/origin HEADs. There were no JavaScript errors. These were local test repositories; no private/live team repository was changed. ZIP imports use multipart form data with field `file`, matching the WRM UI.

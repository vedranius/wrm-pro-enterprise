// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

//go:embed static/*
var staticFiles embed.FS

// AppVersion can be overridden at build time with -ldflags "-X main.AppVersion=..."
var AppVersion = "v14.0.2"

const sessionCookieName = "wrm_session"

var upgrader = websocket.Upgrader{CheckOrigin: checkWSOrigin, ReadBufferSize: 32 * 1024, WriteBufferSize: 64 * 1024}
var db *sql.DB
var encryptionKey []byte

// ─── TYPES ───────────────────────────────────────────

// Connection is the full record including decrypted secrets. It is only used on the
// server; the browser gets a connView without passwords or private keys.
type Connection struct {
	Label      *visualLabel      `json:"label,omitempty"`
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	Protocol   string            `json:"protocol"`
	Host       string            `json:"host"`
	Username   string            `json:"username"`
	AuthMethod string            `json:"auth_method"`
	Password   string            `json:"password"`
	PrivateKey string            `json:"private_key"`
	KeyPath    string            `json:"key_path"`
	FolderID   *int              `json:"folder_id"`
	JumpID     *int              `json:"jump_id"`           // reach this connection through another SSH connection
	ProxyID    *int              `json:"proxy_id"`          // reach it through a saved proxy (after the jump hosts)
	WebPath    string            `json:"web_path"`          // HTTP/HTTPS connections: path of the web interface
	Monitor    *bool             `json:"monitor,omitempty"` // live status checks (nil = unchanged / default on)
	Options    map[string]string `json:"options,omitempty"` // RDP/VNC/Telnet options (nil = unchanged)
	UserID     int               `json:"-"`
	// KEY_REF logs in with a key of the key store, CREDENTIAL with a vault credential.
	KeyID         *int       `json:"key_id,omitempty"`
	CredentialID  *int       `json:"credential_id,omitempty"`
	Tags          []string   `json:"tags,omitempty"`           // nil = unchanged (PUT)
	BMC           *bmcConfig `json:"bmc,omitempty"`            // out-of-band management; nil = unchanged (PUT)
	Notes         *string    `json:"notes,omitempty"`          // runbook; nil = unchanged (PUT)
	Temporary     *bool      `json:"temporary,omitempty"`      // quick connection; PUT false saves it
	KeyRef        string     `json:"key_ref,omitempty"`        // export / import: name of the key
	CredentialRef string     `json:"credential_ref,omitempty"` // export / import: name of the credential
	ProxyRef      string     `json:"proxy_ref,omitempty"`      // export / import: name of the proxy
	authErr       string     // why the key or credential cannot be used (set by resolveConnectionAuth)
	keyCert       string     // SSH user certificate of a stored key (after resolving, ssh_cert.go)
	usedKeyID     int        // stored key the connection logs in with (after resolving)
	usedCredID    int        // vault credential it logs in with (after resolving)
	// JumpID / ProxyID hold the effective route (folder defaults applied). The stored
	// choice: nil = as the folder, -1 = none, > 0 = this one (see applyFolderDefaults).
	jumpRaw, proxyRaw           *int
	jumpInherited, proxyInherit bool
}

type connView struct {
	Label       *visualLabel      `json:"label,omitempty"`
	FolderPath  string            `json:"folder_path,omitempty"`
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Protocol    string            `json:"protocol"`
	Host        string            `json:"host"`
	Username    string            `json:"username"`
	AuthMethod  string            `json:"auth_method"`
	KeyPath     string            `json:"key_path"`
	FolderID    *int              `json:"folder_id"`
	JumpID      *int              `json:"jump_id"`
	ProxyID     *int              `json:"proxy_id"`
	ProxyName   string            `json:"proxy_name,omitempty"`
	JumpChoice  *int              `json:"jump_choice"`  // stored choice: null = folder default, -1 = none
	ProxyChoice *int              `json:"proxy_choice"` // stored choice: null = folder default, -1 = none
	JumpFolder  bool              `json:"jump_inherited,omitempty"`
	ProxyFolder bool              `json:"proxy_inherited,omitempty"`
	WebPath     string            `json:"web_path"`
	Route       string            `json:"route,omitempty"` // jump hosts, e.g. "bastion → dc1-gw"
	HasPassword bool              `json:"has_password"`
	HasKey      bool              `json:"has_private_key"`
	Tunnels     int               `json:"tunnels"`           // configured port forwards
	Monitor     bool              `json:"monitor"`           // included in the live up/down status
	Options     map[string]string `json:"options,omitempty"` // remote desktop options
	KeyID       *int              `json:"key_id,omitempty"`
	KeyName     string            `json:"key_name,omitempty"`
	CredID      *int              `json:"credential_id,omitempty"`
	CredName    string            `json:"credential_name,omitempty"`
	CredUser    string            `json:"credential_user,omitempty"`
	Tags        []string          `json:"tags"`
	Source      string            `json:"source,omitempty"` // inventory it was imported from ("netbox")
	BMC         *bmcConfig        `json:"bmc,omitempty"`
	Temporary   bool              `json:"temporary,omitempty"` // quick connection (deleted when unused)
	HasNotes    bool              `json:"has_notes,omitempty"`
	Notes       string            `json:"notes,omitempty"` // only in GET /api/connections/{id}
}

func (c Connection) view() connView {
	v := connView{ID: c.ID, Name: c.Name, Protocol: c.Protocol, Host: c.Host, Username: c.Username, AuthMethod: c.AuthMethod,
		KeyPath: c.KeyPath, FolderID: c.FolderID, JumpID: c.JumpID, ProxyID: c.ProxyID, WebPath: c.WebPath, HasPassword: c.Password != "", HasKey: c.PrivateKey != "",
		KeyID: c.KeyID, CredID: c.CredentialID, Tags: []string{}, JumpChoice: c.jumpRaw, ProxyChoice: c.proxyRaw,
		JumpFolder: c.jumpInherited, ProxyFolder: c.proxyInherit}
	if needsRoute(c) {
		v.Route = jumpPath(c)
	}
	if c.ProxyID != nil && *c.ProxyID > 0 {
		db.QueryRow(`SELECT name FROM proxies WHERE id=?`, *c.ProxyID).Scan(&v.ProxyName)
	}
	v.Label = loadVisualLabel("connection", c.ID)
	v.FolderPath = folderPathOf(c.UserID, c.FolderID)
	v.Monitor = true
	if c.ID > 0 {
		var mon int
		var opts, tags, ext, temp string
		if db.QueryRow(`SELECT (SELECT COUNT(*) FROM connection_tunnels WHERE conn_id=?), COALESCE(monitor,1), COALESCE(options,''),
			COALESCE((SELECT name FROM ssh_keys WHERE id=connections.key_id),''), COALESCE((SELECT name FROM credentials WHERE id=connections.credential_id),''),
			COALESCE((SELECT username FROM credentials WHERE id=connections.credential_id),''), COALESCE(tags,''), COALESCE(ext_id,''),
			COALESCE(temp_until,''), COALESCE(notes,'')<>'' FROM connections WHERE id=?`, c.ID, c.ID).
			Scan(&v.Tunnels, &mon, &opts, &v.KeyName, &v.CredName, &v.CredUser, &tags, &ext, &temp, &v.HasNotes) == nil {
			v.Temporary = temp != ""
			v.Monitor = mon == 1
			v.Tags = parseTags(tags)
			if b, ok := loadBMC(c.ID); ok {
				v.BMC = b.view()
			}
			if i := strings.IndexByte(ext, ':'); i > 0 {
				v.Source = ext[:i]
			}
			if opts != "" && opts != "{}" {
				json.Unmarshal([]byte(opts), &v.Options)
			}
		}
	}
	return v
}

type Folder struct {
	Label     *visualLabel `json:"label,omitempty"`
	SortOrder int          `json:"sort_order"`
	ParentID  *int         `json:"parent_id"`
	Path      string       `json:"path,omitempty"`
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	JumpID    *int         `json:"jump_id"`  // default jump host of its connections
	ProxyID   *int         `json:"proxy_id"` // default proxy of its connections
	JumpName  string       `json:"jump_name,omitempty"`
	ProxyName string       `json:"proxy_name,omitempty"`
	ProxyRef  string       `json:"proxy_ref,omitempty"` // export / import: name of the proxy
	AutoSort  bool         `json:"auto_sort,omitempty"` // subfolders kept sorted by name (all levels)
}

type FileItem struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	IsLink  bool   `json:"is_link,omitempty"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"mtime"`
}

type Session struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	CreatedAt     string `json:"created_at"`
	Locked        bool   `json:"locked"`
	Pane1ConnID   *int   `json:"pane1_conn_id"`
	Pane1Mode     string `json:"pane1_mode"`
	Pane2ConnID   *int   `json:"pane2_conn_id"`
	Pane2Mode     string `json:"pane2_mode"`
	Pane1ConnName string `json:"pane1_conn_name,omitempty"`
	Pane2ConnName string `json:"pane2_conn_name,omitempty"`
	// Layout is a JSON document describing the open windows (connection, mode, path,
	// position/snap) so a saved session can be restored with one click.
	Layout string `json:"layout"`
}

const maxSessionLayout = 256 * 1024

// ─── BROADCAST HUB ───────────────────────────────────

// Hub delivers live events to the /ws/events sockets. Messages are addressed to one user
// (userID > 0) so one account never sees another account's events.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]int
}

var hub = &Hub{clients: make(map[chan []byte]int)}

func (h *Hub) subscribe(userID int) chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.clients[ch] = userID
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

func (h *Hub) sendTo(userID int, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, uid := range h.clients {
		if uid != userID {
			continue
		}
		select {
		case ch <- msg:
		default:
		}
	}
}

// sendAll sends msg to every signed-in user (e.g. a shared snippet changed).
func (h *Hub) sendAll(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, uid := range h.clients {
		if uid <= 0 {
			continue
		}
		select {
		case ch <- msg:
		default:
		}
	}
}

func broadcastSessionUpdate(userID int) { hub.sendTo(userID, []byte(`{"type":"sessions_changed"}`)) }

func jsonMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ─── MAIN ────────────────────────────────────────────

// newRouter builds the HTTP handler with every route and the security middleware.
func newRouter() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", staticHandler())
	mux.HandleFunc("/favicon.ico", faviconHandler)
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/share/", sharePageHandler)
	mux.HandleFunc("/api/version", versionHandler)
	// accounts & sign-in
	mux.HandleFunc("/api/auth/config", apiAuthConfigHandler)
	mux.HandleFunc("/api/auth/register", apiAuthRegisterHandler)
	mux.HandleFunc("/api/auth/login", apiAuthLoginHandler)
	mux.HandleFunc("/api/auth/mfa", apiAuthMFAHandler)
	mux.HandleFunc("/api/auth/webauthn/register/begin", apiAuthWebAuthnRegisterBegin)
	mux.HandleFunc("/api/auth/webauthn/register/complete", apiAuthWebAuthnRegisterComplete)
	mux.HandleFunc("/api/auth/webauthn/login/begin", apiAuthWebAuthnLoginBegin)
	mux.HandleFunc("/api/auth/webauthn/login/complete", apiAuthWebAuthnLoginComplete)
	mux.HandleFunc("/api/auth/lock/verify", apiAuthLockVerifyHandler)
	mux.HandleFunc("/api/auth/lock", apiAuthLockHandler)
	mux.HandleFunc("/api/auth/lock/pin", apiAuthLockPinHandler)
	mux.HandleFunc("/api/auth/logout", apiAuthLogoutHandler)
	mux.HandleFunc("/api/auth/me", apiAuthMeHandler)
	mux.HandleFunc("/api/auth/keepalive", apiAuthKeepaliveHandler)
	mux.HandleFunc("/api/auth/password", apiAuthPasswordHandler)
	mux.HandleFunc("/api/auth/2fa/", apiAuth2FAHandler)
	mux.HandleFunc("/api/auth/sessions", apiAuthSessionsHandler)
	mux.HandleFunc("/api/auth/sessions/", apiAuthSessionsHandler)
	mux.HandleFunc("/api/users", apiUsersHandler)
	// connections, folders, workspace sessions
	mux.HandleFunc("/api/connections", apiConnectionsHandler)
	mux.HandleFunc("/api/connections/bulk", apiConnectionsBulkHandler)
	mux.HandleFunc("/api/connections/test", apiConnectionTestHandler)
	mux.HandleFunc("/api/connections/quick", apiQuickConnectHandler)
	mux.HandleFunc("/api/nettools", apiNetToolsHandler)
	mux.HandleFunc("/manifest.webmanifest", manifestHandler)
	mux.HandleFunc("/sw.js", serviceWorkerHandler)
	mux.HandleFunc("/api/connections/", apiConnectionByIDHandler)
	mux.HandleFunc("/api/hostkeys/accept", apiHostKeyAcceptHandler)
	mux.HandleFunc("/api/bitwarden", apiBitwardenHandler)
	mux.HandleFunc("/api/bitwarden/", apiBitwardenHandler)
	mux.HandleFunc("/api/folders", apiFoldersHandler)
	mux.HandleFunc("/api/folders/", apiFolderByIDHandler)
	mux.HandleFunc("/api/folders/reorder", apiFolderReorderHandler)
	mux.HandleFunc("/api/folders/sort", apiFolderSortHandler)
	mux.HandleFunc("/api/sessions", apiSessionsHandler)
	mux.HandleFunc("/api/sessions/", apiSessionByIDHandler)
	mux.HandleFunc("/api/config/export", apiExportHandler)
	mux.HandleFunc("/api/config/import", apiImportHandler)
	mux.HandleFunc("/api/config/import/", apiImportExternalHandler)
	// sharing & collaboration
	mux.HandleFunc("/api/shares", apiSharesHandler)
	mux.HandleFunc("/api/shares/", apiSharesByIDHandler)
	mux.HandleFunc("/api/share/", apiShareByTokenHandler)
	mux.HandleFunc("/api/shared", apiSharedHandler)
	// administration
	mux.HandleFunc("/api/admin/users", apiAdminUsersHandler)
	mux.HandleFunc("/api/admin/users/", apiAdminUserByIDHandler)
	mux.HandleFunc("/api/admin/settings", apiAdminSettingsHandler)
	mux.HandleFunc("/api/admin/security", apiAdminSecurityHandler)
	mux.HandleFunc("/api/admin/security/", apiAdminSecurityHandler)
	mux.HandleFunc("/api/admin/teams", apiAdminTeamsHandler)
	mux.HandleFunc("/api/admin/teams/", apiAdminTeamsHandler)
	mux.HandleFunc("/api/teams", apiTeamsHandler)
	mux.HandleFunc("/api/auth/step-up", apiAuthStepUpHandler)
	mux.HandleFunc("/api/auth/step-up/webauthn/begin", apiAuthStepUpWebAuthnBegin)
	mux.HandleFunc("/api/auth/step-up/webauthn/complete", apiAuthStepUpWebAuthnComplete)
	mux.HandleFunc("/api/auth/oidc/start", apiOIDCStartHandler)
	mux.HandleFunc("/api/admin/lockdown", apiAdminLockdownHandler)
	mux.HandleFunc("/api/admin/backup", apiAdminBackupHandler)
	mux.HandleFunc("/api/admin/siem", apiAdminSIEMHandler)
	mux.HandleFunc("/api/admin/siem/", apiAdminSIEMHandler)
	mux.HandleFunc("/api/admin/ldap", apiAdminLDAPHandler)
	mux.HandleFunc("/api/admin/ldap/", apiAdminLDAPHandler)
	mux.HandleFunc("/api/me/ldap", apiMeLDAPHandler)
	mux.HandleFunc("/api/me/ldap/", apiMeLDAPHandler)
	mux.HandleFunc("/api/admin/scim", apiAdminSCIMHandler)
	mux.HandleFunc("/api/admin/scim/", apiAdminSCIMHandler)
	mux.HandleFunc("/scim/v2/", scimHandler)
	mux.HandleFunc("/api/auth/oidc/callback", apiOIDCCallbackHandler)
	mux.HandleFunc("/api/auth/oidc/result", apiOIDCResultHandler)
	mux.HandleFunc("/saml/metadata", samlMetadataHandler)
	mux.HandleFunc("/saml/acs", samlACSHandler)
	mux.HandleFunc("/api/auth/saml/start", apiSAMLStartHandler)
	mux.HandleFunc("/api/admin/oidc", apiAdminOIDCHandler)
	mux.HandleFunc("/api/admin/oidc/", apiAdminOIDCHandler)
	mux.HandleFunc("/api/me/identities", apiMeIdentitiesHandler)
	mux.HandleFunc("/api/me/identities/", apiMeIdentitiesHandler)
	mux.HandleFunc("/api/admin/audit", apiAdminAuditHandler)
	mux.HandleFunc("/api/admin/audit/verify", apiAdminAuditHandler)
	mux.HandleFunc("/api/admin/transfers", apiAdminTransfersHandler)
	mux.HandleFunc("/api/admin/file-edits", apiAdminFileEditsHandler)
	mux.HandleFunc("/api/admin/file-edits/", apiAdminFileEditsHandler)
	mux.HandleFunc("/api/status", apiStatusHandler)
	mux.HandleFunc("/api/notify", apiNotifyHandler)
	mux.HandleFunc("/api/notify/test", apiNotifyHandler)
	mux.HandleFunc("/api/admin/notify/channels", apiAdminNotifyHandler)
	mux.HandleFunc("/api/admin/notify/channels/", apiAdminNotifyHandler)
	mux.HandleFunc("/api/status/check", apiStatusHandler)
	mux.HandleFunc("/api/inventory/", apiInventoryHandler)
	mux.HandleFunc("/gitlab-relay", gitRelayWorkerHandler)
	mux.HandleFunc("/api/monitoring", apiMonitoringHandler)
	mux.HandleFunc("/api/monitoring/", apiMonitoringHandler)
	mux.HandleFunc("/api/addons", apiAddonsHandler)
	mux.HandleFunc("/api/addons/", apiAddonsHandler)
	mux.HandleFunc("/api/git", apiGitHandler)
	mux.HandleFunc("/api/git/", apiGitHandler)
	mux.HandleFunc("/api/git/medium", apiMediumHandler)
	mux.HandleFunc("/api/git/medium/", apiMediumHandler)
	mux.HandleFunc("/medium/", mediumProxyHandler)
	mux.HandleFunc("/api/hooks/git/", gitHookHandler)
	mux.HandleFunc("/api/keys", apiKeysHandler)
	mux.HandleFunc("/api/keys/", apiKeysHandler)
	mux.HandleFunc("/api/credentials", apiCredentialsHandler)
	mux.HandleFunc("/api/credentials/checkouts", apiCheckoutsHandler)
	mux.HandleFunc("/api/credentials/checkouts/", apiCheckoutsHandler)
	mux.HandleFunc("/api/credentials/", apiCredentialsHandler)
	mux.HandleFunc("/api/proxies", apiProxiesHandler)
	mux.HandleFunc("/api/proxies/", apiProxiesHandler)
	mux.HandleFunc("/api/snippets", apiSnippetsHandler)
	mux.HandleFunc("/api/me/ui/", apiUserUIHandler)
	mux.HandleFunc("/api/me/events", apiUserEventsHandler)
	mux.HandleFunc("/api/me/events/", apiUserEventsHandler)
	mux.HandleFunc("/api/workboard/metrics", apiWorkboardMetrics)
	mux.HandleFunc("/api/snippets/", apiSnippetsHandler)
	mux.HandleFunc("/api/bookmarks", apiBookmarksHandler)
	mux.HandleFunc("/api/bookmarks/", apiBookmarksHandler)
	mux.HandleFunc("/api/tunnels", apiTunnelsHandler)
	mux.HandleFunc("/api/tunnels/", apiTunnelsHandler)
	mux.HandleFunc("/api/ai/", apiAIHandler)
	mux.HandleFunc("/api/admin/ai/", apiAdminAIHandler)
	mux.HandleFunc("/api/mcp/", apiMCPHandler)
	mux.HandleFunc("/api/admin/mcp/", apiAdminMCPHandler)
	mux.HandleFunc("/api/admin/opencode", apiAdminOpencodeHandler)
	mux.HandleFunc("/api/admin/opencode/", apiAdminOpencodeHandler)
	mux.HandleFunc("/mcp", mcpHandler)
	mux.HandleFunc("/.well-known/oauth-protected-resource", mcpProtectedResourceHandler)
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", mcpProtectedResourceHandler)
	mux.HandleFunc("/.well-known/oauth-authorization-server", mcpAuthServerMetadataHandler)
	mux.HandleFunc("/.well-known/oauth-authorization-server/mcp", mcpAuthServerMetadataHandler)
	mux.HandleFunc("/oauth/register", mcpRegisterHandler)
	mux.HandleFunc("/oauth/authorize", mcpAuthorizeHandler)
	mux.HandleFunc("/oauth/token", mcpTokenHandler)
	mux.HandleFunc("/oauth/revoke", mcpRevokeHandler)
	mux.HandleFunc("/api/recordings", apiRecordingsHandler)
	mux.HandleFunc("/api/recordings/", apiRecordingsHandler)
	mux.HandleFunc("/api/admin/known-hosts", apiAdminKnownHostsHandler)
	mux.HandleFunc("/api/admin/known-hosts/", apiAdminKnownHostsHandler)
	mux.HandleFunc("/api/admin/guacd", apiAdminGuacdHandler)
	mux.HandleFunc("/api/admin/status", apiAdminStatusHandler)
	mux.HandleFunc("/api/update", apiUpdateHandler)
	mux.HandleFunc("/api/update/", apiUpdateHandler)
	mux.HandleFunc("/api/admin/terminals/", apiAdminTerminalsHandler)
	mux.HandleFunc("/api/admin/shares", apiAdminSharesHandler)
	// remote files
	mux.HandleFunc("/api/remote/list", listRemoteFilesHandler)
	mux.HandleFunc("/api/remote/list-recursive", listRemoteRecursiveHandler)
	mux.HandleFunc("/api/remote/download", downloadRemoteFileHandler)
	mux.HandleFunc("/api/remote/preview", previewRemoteFileHandler)
	mux.HandleFunc("/api/remote/download-dir", downloadRemoteDirHandler)
	mux.HandleFunc("/api/remote/upload", uploadRemoteFileHandler)
	mux.HandleFunc("/api/remote/mkdir", mkdirRemoteHandler)
	mux.HandleFunc("/api/remote/chmod", chmodRemoteHandler)
	mux.HandleFunc("/api/remote/delete", deleteRemoteHandler)
	mux.HandleFunc("/api/remote/rename", renameRemoteHandler)
	mux.HandleFunc("/api/remote/transfer", transferRemoteHandler)
	mux.HandleFunc("/api/remote/search", searchRemoteHandler)
	// websockets
	mux.HandleFunc("/ws/ssh", sshHandler)
	mux.HandleFunc("/ws/desktop", desktopWSHandler)
	mux.HandleFunc("/ws/share/", shareWSHandler)
	mux.HandleFunc("/ws/events", eventsHandler)
	return securityMiddleware(mux)
}

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	resetPassword := flag.String("reset-password", "", "set a new temporary password for `USER` (e.g. a locked-out administrator) and exit")
	reset2FA := flag.Bool("reset-2fa", false, "together with -reset-password: also turn off two-factor authentication")
	healthcheck := flag.Bool("healthcheck", false, "check /healthz of the server running on this machine and exit (for container health checks)")
	installSvc := flag.Bool("install-service", false, "install WRM as a service that starts at boot (systemd, launchd, rc.d or a Windows service), start it and exit")
	uninstallSvc := flag.Bool("uninstall-service", false, "stop and remove the WRM service and exit (the data stays)")
	svcStatus := flag.Bool("service-status", false, "show whether the WRM service is installed and running, and exit")
	noSvcPrompt := flag.Bool("no-service-prompt", false, "do not ask on the console whether to install WRM as a service")
	svcName := flag.String("service-name", "wrm", "`NAME` of the service for -install-service, -uninstall-service and -service-status")
	svcScope := flag.String("service-scope", "", "macOS: `system` (LaunchDaemon, starts at boot) or user (LaunchAgent, starts at login)")
	svcUser := flag.String("service-user", "", "Unix `ACCOUNT` the service runs as (default: the user who runs the installer, or SUDO_USER)")
	flag.StringVar(&serviceMode, "service", "", "set by the service definition: systemd, launchd, rcd, rcd-openbsd or windows (not for interactive use)")
	envFile := flag.String("env-file", "", "read environment variables (KEY=value lines) from `FILE` before starting; variables already set win")
	workDir := flag.String("workdir", "", "change to `DIR` before starting")
	mcpStdio := flag.Bool("mcp-stdio", false, "run as a stdio bridge for MCP clients to the WRM server at -url (with -token) and exit when stdin closes")
	mcpURL := flag.String("url", "", "with -mcp-stdio: the `URL` of the WRM server's MCP endpoint, e.g. https://wrm.example.com/mcp (or WRM_MCP_URL)")
	mcpToken := flag.String("token", "", "with -mcp-stdio: the AI connection `TOKEN` from Settings → AI connections (or -token-file, WRM_MCP_TOKEN)")
	mcpTokenFile := flag.String("token-file", "", "with -mcp-stdio: read the token from `FILE`")
	mcpCA := flag.String("ca-file", "", "with -mcp-stdio: trust the CA or self-signed certificate in `FILE` (PEM) for the WRM server")
	setupDocker := flag.Bool("setup-docker", false, "Linux, as root: install Docker Engine for Medium, let the WRM account (-service-user, default: the WRM service's user) use it, restart the WRM service and exit")
	setupGuacd := flag.Bool("setup-guacd", false, "Windows/macOS: start the guacd container for remote desktop in Docker Desktop and exit")
	gitlabRelay := flag.Bool("gitlab-relay", false, "run a client GitLab relay on this computer, using the exact -url relay job URL from WRM")
	gitlabURL := flag.String("gitlab-url", "", "the selected GitLab origin for the client relay")
	relayKeyFile := flag.String("relay-key-file", "", "read the short-lived WRM relay job key from FILE")
	relayLive := flag.Bool("relay-live", true, "keep the GitLab client relay online for uncached read-only API operations until the job expires")
	gitlabTokenFile := flag.String("gitlab-token-file", "", "read the client GitLab access token from FILE (or WRM_GITLAB_TOKEN)")
	releaseKeygen := flag.String("release-keygen", "", "create a release signing key pair: write the private key to `FILE`, print the public key, and exit")
	releaseSign := flag.String("release-sign", "", "sign `FILE` (SHA256SUMS.txt) with the release key from "+releaseSigningKeyEnv+" or -release-key-file, write FILE.sig and exit")
	releaseKeyFile := flag.String("release-key-file", "", "with -release-sign: read the private release key from `FILE`")
	releaseVerify := flag.String("release-verify", "", "check the signature FILE.sig of `FILE` (a downloaded SHA256SUMS.txt) and the SHA-256 of the listed files next to it, and exit")
	releasePublicKey := flag.String("release-public-key", "", "with -release-verify: also trust this public `KEY` (wrm-ed25519:…)")
	lockdownMode := flag.String("lockdown", "", "`on`, off or status: turn the lockdown on or off (only break-glass administrators can sign in, AI stopped, shares paused) and exit; a running WRM applies it within seconds")
	lockdownReason := flag.String("lockdown-reason", "", "with -lockdown on: the `REASON` shown to everyone")
	backup := flag.String("backup", "", "write a consistent database backup to `FILE` (plus FILE.key when the key is a file) and exit")
	backupWithoutKey := flag.Bool("backup-without-key", false, "with -backup: do not copy the encryption key next to the backup")
	restore := flag.String("restore", "", "restore the database from the backup `FILE` and exit (run while WRM is stopped; restore the matching encryption key yourself)")
	flag.StringVar(&initialSecurityLevel, "security-level", "", "`LEVEL` home, business or enterprise: the security level to start with while none was chosen yet (also with -install-service)")
	addonLint := flag.String("addon-lint", "", "check the addon in `DIR` (or a .zip) as an install would, list what its sandbox refuses, and exit (status 1 on errors)")
	addonLintJSON := flag.Bool("addon-lint-json", false, "with -addon-lint: print the report as JSON")
	addonNew := flag.String("addon-new", "", "create an addon from `TEMPLATE` (blank, fetch-and-render, grafana-panel, wasm-hook) in the folder named by the next argument (default ./TEMPLATE) and exit")
	addonNewID := flag.String("addon-id", "", "with -addon-new: the addon `ID` written into addon.json")
	addonDev := flag.String("addon-dev", "", "serve the addon in `DIR` on 127.0.0.1 with the real sandbox, a stand-in for WRM and reload on change")
	addonDevPort := flag.Int("addon-dev-port", 8790, "with -addon-dev: the `PORT` (0 picks a free one)")
	addonDevPrivate := flag.Bool("addon-dev-private", false, "with -addon-dev: allow private and local addresses for the listed origins")
	flag.Parse()
	switch {
	case *addonLint != "":
		os.Exit(runAddonLint(*addonLint, *addonLintJSON, os.Stdout))
	case *addonNew != "":
		os.Exit(runAddonNew(*addonNew, *addonNewID, flag.Arg(0), os.Stdout))
	case *addonDev != "":
		os.Exit(runAddonDev(*addonDev, *addonDevPort, *addonDevPrivate))
	case *releaseKeygen != "":
		os.Exit(runReleaseKeygen(*releaseKeygen, os.Stdout))
	case *releaseSign != "":
		os.Exit(runReleaseSign(*releaseSign, *releaseKeyFile, os.Stdout))
	case *releaseVerify != "":
		os.Exit(runReleaseVerify(*releaseVerify, *releasePublicKey, os.Stdout))
	}
	if *gitlabRelay {
		os.Exit(runGitLabRelayClient(*mcpURL, *gitlabURL, *relayKeyFile, *gitlabTokenFile, *mcpCA, os.Stdout, *relayLive))
	}
	if *showVersion {
		fmt.Println(AppVersion)
		return
	}
	if *mcpStdio {
		os.Exit(runMCPStdio(*mcpURL, *mcpToken, *mcpTokenFile, *mcpCA, os.Stdin, os.Stdout))
	}
	if *workDir != "" {
		if err := os.Chdir(*workDir); err != nil {
			log.Fatalf("-workdir: %v", err)
		}
	}
	if *envFile != "" {
		if err := loadEnvFile(*envFile); err != nil {
			log.Fatalf("-env-file: %v", err)
		}
	}
	if *healthcheck {
		os.Exit(runHealthcheck())
	}
	currentServiceName = *svcName
	if *setupDocker {
		os.Exit(runSetupDocker(*svcUser, *svcName, os.Stdout))
	}
	if *setupGuacd {
		os.Exit(runSetupGuacd(os.Stdout))
	}
	opts := serviceOptions{Name: *svcName, Scope: *svcScope, User: *svcUser}
	if *installSvc && os.Getenv("WRM_APPIMAGE") == "1" {
		fmt.Fprintln(os.Stderr, "For boot services, download the standalone Linux binary and run -install-service from its permanent location.")
		os.Exit(1)
	}
	if initialSecurityLevel != "" {
		l, ok := validSecurityLevel(initialSecurityLevel)
		if !ok {
			fmt.Fprintln(os.Stderr, "-security-level: use home, business or enterprise")
			os.Exit(2)
		}
		initialSecurityLevel = l
		if *installSvc {
			os.Setenv(initialSecurityLevelEnv, l) // copied into the service's environment file
		}
	}
	if *installSvc || *uninstallSvc || *svcStatus {
		os.Exit(runServiceCommand(*installSvc, *uninstallSvc, *svcStatus, opts))
	}
	if *resetPassword != "" {
		initDB()
		loadAppSettings()
		initServerSecret()
		initEncryptionKey()
		migrateSessionTokens()
		ensureAdminExists()
		if err := resetPasswordCLI(*resetPassword, *reset2FA); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *lockdownMode != "" {
		initDB()
		loadAppSettings()
		if err := lockdownCLI(*lockdownMode, *lockdownReason); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *backup != "" {
		if err := backupCLI(*backup, !*backupWithoutKey); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *restore != "" {
		if err := restoreCLI(*restore); err != nil {
			log.Fatal(err)
		}
		return
	}
	if handled, code := runAsWindowsService(*svcName, serve); handled {
		os.Exit(code)
	}
	if shouldPromptService(*noSvcPrompt, *svcName) && promptService(opts) {
		return
	}
	os.Exit(serve(nil))
}

// serve runs the server until a signal, a stop of the Windows service (stop) or a restart
// request, and returns the exit code.
func serve(stop <-chan struct{}) int {
	if serviceMode != "" {
		openServiceLog()
		if switchToNewerBinary() {
			return exitRestart
		}
	}
	cleanupReplacedBinary()
	initDB()
	mdCleanupBuildDirs()
	loadAppSettings()
	initServerSecret()
	initEncryptionKey()
	migrateSessionTokens()
	ensureAdminExists()
	applySecurityLevelAtStart(initialSecurityLevel)
	mdMigrateDefaultEngine()
	log.Printf("Web Remote Manager %s starting%s", AppVersion, map[bool]string{true: " (service: " + serviceMode + ")"}[serviceMode != ""])
	log.Printf("Encryption key for stored secrets: %s", encryptionKeySource)
	startTURN()
	tunnelMgr.startAlways()
	go runMonitoring()
	go statusMon.run()
	go notifier.run()
	go telemetryLoop()
	go backupLoop()
	go runCredentialReminders()
	go runQuickCleanup()
	go runGitMonitor()
	go runBitwardenJanitor()
	go runGitScheduler()
	go runUpdateChecker()
	go runAIJanitor()
	go runMCPJanitor()
	go opencodeCleanup()
	go guacdAutoStartLoop()

	recoverTerminalSessions()
	if dir := recordingsDir(); settingBool("session_recording") {
		log.Printf("Session recording is on (recordings in %s)", dir)
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		addr = ":" + port
	}

	// Background maintenance: expired sign-in sessions every hour, audit retention daily.
	go func() {
		for i := 0; ; i++ {
			cleanupAuthSessions()
			if i%24 == 0 {
				cleanupAuditLog()
			}
			time.Sleep(time.Hour)
		}
	}()
	go oidcRevalidateLoop()
	go ldapSyncLoop()
	go lockdownWatchLoop()
	go siemLoop()

	srv := &http.Server{
		Addr: addr, Handler: newRouter(),
		ReadHeaderTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10,
	}
	certFile, keyFile := os.Getenv("HTTPS_CERT_FILE"), os.Getenv("HTTPS_KEY_FILE")
	if (certFile == "" || keyFile == "") && os.Getenv("HTTPS_SELF_SIGNED") == "1" {
		var err error
		if certFile, keyFile, err = ensureSelfSignedCert(); err != nil {
			log.Fatalf("self-signed certificate: %v", err)
		}
	}
	tlsOn := certFile != "" && keyFile != ""
	if !tlsOn {
		log.Printf("NOTE: serving plain HTTP. Use HTTPS (HTTPS_CERT_FILE/HTTPS_KEY_FILE, HTTPS_SELF_SIGNED=1 or a reverse proxy) — browsers allow the microphone for voice calls only over HTTPS.")
	}
	if getSetting("registration") == "open" {
		log.Printf("NOTE: self-registration is open — anyone who can reach this server can create an account.")
	}

	shutdownDone := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		select {
		case <-sig:
			log.Printf("Shutting down…")
		case <-stop:
			log.Printf("Service stop requested, shutting down…")
		case <-restartState.ch:
			log.Printf("Restarting…")
		}
		stopTURN()
		mediumStopAll()
		opencodeStopAll()
		tunnelMgr.stopAll("WRM was stopped")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		close(shutdownDone)
	}()

	var err error
	if tlsOn {
		log.Printf("Listening on %s (HTTPS, certificate %s) — open %s in your browser", addr, certFile, browserURL(addr, true))
		err = srv.ListenAndServeTLS(certFile, keyFile)
	} else {
		log.Printf("Listening on %s — open %s in your browser", addr, browserURL(addr, false))
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-shutdownDone
	db.Close()
	if ok, target := restartRequested(); ok {
		return finishRestart(target)
	}
	return 0
}

// serveIndex serves the single-page app with a validator and (when the browser accepts
// it) gzip. The embedded file never changes at runtime, so the ETag can be cached.
func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	serveEmbedded(w, r, "static/index.html")
}

var (
	staticETags sync.Map // name -> ETag
	staticGzip  sync.Map // name -> gzipped bytes
)

func staticETag(name string, data []byte) string {
	if v, ok := staticETags.Load(name); ok {
		return v.(string)
	}
	sum := sha256.Sum256(data)
	et := `"` + hex.EncodeToString(sum[:12]) + `"`
	staticETags.Store(name, et)
	return et
}

func gzipStatic(name string, data []byte) []byte {
	if v, ok := staticGzip.Load(name); ok {
		return v.([]byte)
	}
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write(data)
	zw.Close()
	out := b.Bytes()
	staticGzip.Store(name, out)
	return out
}

func compressibleType(ct string) bool {
	return strings.HasPrefix(ct, "text/") || strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "json") || strings.Contains(ct, "svg")
}

// serveEmbedded writes one embedded file with an ETag, a 304 path, gzip and an explicit
// Content-Length. embed.FS files have no modification time, so without this every reload
// transferred the whole 1.2 MB page again.
func serveEmbedded(w http.ResponseWriter, r *http.Request, name string) {
	data, err := staticFiles.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if strings.HasSuffix(name, ".html") || ct == "" {
		ct = "text/html; charset=utf-8"
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	et := staticETag(name, data)
	h.Set("ETag", et)
	h.Set("Vary", "Accept-Encoding")
	if inm := r.Header.Get("If-None-Match"); inm != "" && (inm == et || inm == "*") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := data
	if compressibleType(ct) && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		body = gzipStatic(name, data)
		h.Set("Content-Encoding", "gzip")
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	// #nosec G705 -- the body is an embedded file from staticFiles, never request data
	w.Write(body)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "" {
		serveIndex(w, r)
		return
	}
	http.NotFound(w, r)
}

func sharePageHandler(w http.ResponseWriter, r *http.Request) { serveIndex(w, r) }

// versionHandler also names an available update (for the sign-in page's version badge).
func versionHandler(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{"version": AppVersion}
	if info := lastUpdateInfo(); settingBool("update_check") && updateAvailable(info) {
		out["update"] = info.Latest
	}
	jsonOK(w, out)
}

// healthHandler is for load balancers and monitoring: 200 when the database answers.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"error","database":"unavailable"}`))
		return
	}
	jsonOK(w, map[string]string{"status": "ok", "version": AppVersion})
}

// runHealthcheck queries /healthz of the local server (exit code 0 = healthy).
func runHealthcheck() int {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		addr = ":" + port
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	scheme := "http"
	if (os.Getenv("HTTPS_CERT_FILE") != "" && os.Getenv("HTTPS_KEY_FILE") != "") || os.Getenv("HTTPS_SELF_SIGNED") == "1" {
		scheme = "https"
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		// Loopback check of our own (possibly self-signed) certificate.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}}
	resp, err := client.Get(scheme + "://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthz:", resp.Status)
		return 1
	}
	return 0
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/brand/favicon.ico")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// ─── DATABASE ────────────────────────────────────────

func resolveDBPath() string {
	if p := os.Getenv("DB_PATH"); p != "" {
		return p
	}
	wd, err := os.Getwd()
	if err != nil {
		return "remote_manager.db"
	}
	return filepath.Join(wd, "remote_manager.db")
}

func initDB() {
	dbPath := resolveDBPath()
	var err error
	// PRAGMAs are per connection. A PRAGMA run once only reaches one pooled connection, so
	// both busy_timeout and foreign_keys are part of the DSN; otherwise ON DELETE CASCADE
	// and FK checks work only on whichever connection happened to run the statement.
	dsn := dbPath
	if !strings.ContainsAny(dbPath, "#") {
		sep := "?"
		if strings.Contains(dbPath, "?") {
			sep = "&"
		}
		dsn = dbPath + sep + "_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)"
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("Open DB: %v", err)
	}
	// A small, bounded pool: SQLite allows one writer and any number of readers, and an
	// unbounded pool under a burst only produces "database is locked" storms.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err = db.Ping(); err != nil {
		log.Fatalf("Ping DB: %v", err)
	}
	db.Exec(`PRAGMA journal_mode=WAL`)
	// The database holds password hashes and encrypted secrets: keep it private.
	for _, f := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		os.Chmod(f, 0o600)
	}

	mustExec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL)`)
	migrateWebAuthn()
	migrateCheckouts()
	mustExec(`CREATE TABLE IF NOT EXISTS auth_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token TEXT NOT NULL UNIQUE,
		expires_at TEXT NOT NULL)`)
	mustExec(`CREATE TABLE IF NOT EXISTS folders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		user_id INTEGER DEFAULT NULL)`)
	mustExec(`CREATE TABLE IF NOT EXISTS connections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		protocol TEXT NOT NULL DEFAULT 'SSH',
		host TEXT NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		auth_method TEXT NOT NULL DEFAULT 'PASSWORD',
		password TEXT DEFAULT '',
		private_key TEXT DEFAULT '',
		key_path TEXT DEFAULT '',
		folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL,
		user_id INTEGER DEFAULT NULL)`)
	mustExec(`CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL,
		locked INTEGER NOT NULL DEFAULT 0,
		pane1_conn_id INTEGER REFERENCES connections(id) ON DELETE SET NULL,
		pane1_mode TEXT NOT NULL DEFAULT '',
		pane2_conn_id INTEGER REFERENCES connections(id) ON DELETE SET NULL,
		pane2_mode TEXT NOT NULL DEFAULT '',
		user_id INTEGER DEFAULT NULL)`)
	mustExec(`CREATE TABLE IF NOT EXISTS share_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		password_hash TEXT DEFAULT NULL,
		public INTEGER NOT NULL DEFAULT 1,
		active INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL)`)
	mustExec(`CREATE TABLE IF NOT EXISTS share_items (
		share_id INTEGER NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
		connection_id INTEGER REFERENCES connections(id) ON DELETE CASCADE,
		folder_id INTEGER REFERENCES folders(id) ON DELETE CASCADE)`)
	mustExec(`CREATE TABLE IF NOT EXISTS share_members (
		share_id INTEGER NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE)`)
	ensureTable("share_participants", `CREATE TABLE IF NOT EXISTS share_participants (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		share_id INTEGER NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
		pkey TEXT NOT NULL,
		user_id INTEGER DEFAULT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		banned INTEGER NOT NULL DEFAULT 0,
		first_seen TEXT NOT NULL DEFAULT '',
		last_seen TEXT NOT NULL DEFAULT '',
		last_ip TEXT NOT NULL DEFAULT '',
		UNIQUE(share_id, pkey))`,
		"id", "share_id", "pkey", "user_id", "name", "role", "banned", "first_seen", "last_seen", "last_ip")
	ensureTable("collab_messages", `CREATE TABLE IF NOT EXISTS collab_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		share_id INTEGER NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
		ts TEXT NOT NULL,
		pkey TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 'chat',
		text TEXT NOT NULL DEFAULT '')`,
		"id", "share_id", "ts", "pkey", "name", "kind", "text")
	ensureTable("audit_log", `CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '',
		details TEXT NOT NULL DEFAULT '')`,
		"id", "ts", "user_id", "username", "ip", "action", "target", "details")
	ensureTable("known_hosts", `CREATE TABLE IF NOT EXISTS known_hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL UNIQUE,
		key_type TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		public_key TEXT NOT NULL,
		first_seen TEXT NOT NULL,
		last_seen TEXT NOT NULL,
		added_by TEXT NOT NULL DEFAULT '')`,
		"id", "host", "key_type", "fingerprint", "public_key", "first_seen", "last_seen", "added_by")
	ensureTable("app_settings", `CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL)`,
		"key", "value")
	// v10.1: terminal sessions, recordings and file transfers (audit trail). No foreign keys:
	// audit records must survive the deletion of users, connections and shares.
	ensureTable("terminal_sessions", `CREATE TABLE IF NOT EXISTS terminal_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uid TEXT NOT NULL UNIQUE,
		user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '',
		pkey TEXT NOT NULL DEFAULT '',
		share_id INTEGER DEFAULT NULL,
		share_name TEXT NOT NULL DEFAULT '',
		conn_id INTEGER DEFAULT NULL,
		conn_name TEXT NOT NULL DEFAULT '',
		conn_owner_id INTEGER DEFAULT NULL,
		host TEXT NOT NULL DEFAULT '',
		remote_user TEXT NOT NULL DEFAULT '',
		protocol TEXT NOT NULL DEFAULT 'ssh',
		client_ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		ended_at TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'connecting',
		exit_code INTEGER DEFAULT NULL,
		jump_path TEXT NOT NULL DEFAULT '',
		bytes_out INTEGER NOT NULL DEFAULT 0,
		bytes_in INTEGER NOT NULL DEFAULT 0)`,
		"id", "uid", "user_id", "username", "pkey", "share_id", "share_name", "conn_id", "conn_name", "conn_owner_id",
		"host", "remote_user", "protocol", "client_ip", "user_agent", "started_at", "ended_at", "status", "exit_code",
		"jump_path", "bytes_out", "bytes_in")
	ensureTable("session_recordings", `CREATE TABLE IF NOT EXISTS session_recordings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		format TEXT NOT NULL,
		path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		data_bytes INTEGER NOT NULL DEFAULT 0,
		sha256 TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		input_recorded INTEGER NOT NULL DEFAULT 0,
		truncated INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL)`,
		"id", "session_id", "format", "path", "size_bytes", "data_bytes", "sha256", "duration_ms", "input_recorded", "truncated", "created_at")
	// v10.2: port forwards (tunnels) configured on a connection
	ensureTable("connection_tunnels", `CREATE TABLE IF NOT EXISTS connection_tunnels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conn_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 'local',
		bind_host TEXT NOT NULL DEFAULT '127.0.0.1',
		bind_port INTEGER NOT NULL DEFAULT 0,
		target_host TEXT NOT NULL DEFAULT '',
		target_port INTEGER NOT NULL DEFAULT 0,
		open_scheme TEXT NOT NULL DEFAULT '',
		open_path TEXT NOT NULL DEFAULT '',
		start_mode TEXT NOT NULL DEFAULT 'manual',
		sort INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '')`,
		"id", "conn_id", "user_id", "name", "kind", "bind_host", "bind_port", "target_host", "target_port", "open_scheme", "open_path", "start_mode", "sort", "created_at")
	ensureTable("file_transfers", `CREATE TABLE IF NOT EXISTS file_transfers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		share_id INTEGER DEFAULT NULL,
		direction TEXT NOT NULL,
		src_conn_id INTEGER DEFAULT NULL,
		src_host TEXT NOT NULL DEFAULT '',
		src_path TEXT NOT NULL DEFAULT '',
		dst_conn_id INTEGER DEFAULT NULL,
		dst_host TEXT NOT NULL DEFAULT '',
		dst_path TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER NOT NULL DEFAULT 0,
		sha256 TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'ok',
		error TEXT NOT NULL DEFAULT '')`,
		"id", "ts", "user_id", "username", "client_ip", "share_id", "direction", "src_conn_id", "src_host", "src_path",
		"dst_conn_id", "dst_host", "dst_path", "size_bytes", "sha256", "status", "error")

	// v10.3: saved commands (snippets), optionally run when a terminal connects
	ensureTable("snippets", `CREATE TABLE IF NOT EXISTS snippets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		grp TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL DEFAULT 'all',
		scope_id INTEGER NOT NULL DEFAULT 0,
		auto_run INTEGER NOT NULL DEFAULT 0,
		shared INTEGER NOT NULL DEFAULT 0,
		sort INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "name", "command", "description", "grp", "scope", "scope_id", "auto_run", "shared", "sort", "created_at", "updated_at")

	// v10.10: folder bookmarks (server-specific, folder, tag and global)
	ensureTable("bookmarks", `CREATE TABLE IF NOT EXISTS bookmarks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL DEFAULT 'global',
		scope_id INTEGER NOT NULL DEFAULT 0,
		tag TEXT NOT NULL DEFAULT '',
		color TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '',
		start_dir INTEGER NOT NULL DEFAULT 0,
		shared INTEGER NOT NULL DEFAULT 0,
		sort INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "name", "path", "scope", "scope_id", "tag", "color", "note", "start_dir", "shared", "sort", "created_at", "updated_at")

	// v10.5: SSH key store and credentials vault (secrets encrypted like connection secrets)
	ensureTable("ssh_keys", `CREATE TABLE IF NOT EXISTS ssh_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		key_type TEXT NOT NULL DEFAULT '',
		bits INTEGER NOT NULL DEFAULT 0,
		public_key TEXT NOT NULL DEFAULT '',
		private_key TEXT NOT NULL DEFAULT '',
		fingerprint TEXT NOT NULL DEFAULT '',
		comment TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "name", "key_type", "bits", "public_key", "private_key", "fingerprint", "comment", "created_at", "last_used_at")
	ensureTable("ssh_key_deployments", `CREATE TABLE IF NOT EXISTS ssh_key_deployments (
		key_id INTEGER NOT NULL,
		conn_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL DEFAULT 0,
		deployed_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (key_id, conn_id))`,
		"key_id", "conn_id", "user_id", "deployed_at")
	ensureTable("credentials", `CREATE TABLE IF NOT EXISTS credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT '',
		password TEXT NOT NULL DEFAULT '',
		key_id INTEGER DEFAULT NULL,
		description TEXT NOT NULL DEFAULT '',
		hosts TEXT NOT NULL DEFAULT '',
		shared_all INTEGER NOT NULL DEFAULT 0,
		rotate_days INTEGER NOT NULL DEFAULT 0,
		rotated_at TEXT NOT NULL DEFAULT '',
		pending_password TEXT NOT NULL DEFAULT '',
		rotation_status TEXT NOT NULL DEFAULT '',
		dual_control INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "owner_id", "name", "username", "password", "key_id", "description", "hosts", "shared_all", "rotate_days", "rotated_at",
		"pending_password", "rotation_status", "dual_control", "created_at", "updated_at")
	ensureTable("credential_grants", `CREATE TABLE IF NOT EXISTS credential_grants (
		credential_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		PRIMARY KEY (credential_id, user_id))`,
		"credential_id", "user_id")

	// v10.6: inventory sources (NetBox address and encrypted token per user)
	ensureTable("inventory_sources", `CREATE TABLE IF NOT EXISTS inventory_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		kind TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		token TEXT NOT NULL DEFAULT '',
		options TEXT NOT NULL DEFAULT '',
		last_sync_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "kind", "url", "token", "options", "last_sync_at")

	// v10.9: notifications (channels configured by administrators, subscriptions per user)
	ensureTable("notify_channels", `CREATE TABLE IF NOT EXISTS notify_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT '',
		config TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		user_address INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		last_sent_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '')`,
		"id", "name", "kind", "config", "enabled", "user_address", "created_at", "updated_at", "last_sent_at", "last_error")
	ensureTable("notify_subscriptions", `CREATE TABLE IF NOT EXISTS notify_subscriptions (
		user_id INTEGER NOT NULL,
		event TEXT NOT NULL,
		channel_id INTEGER NOT NULL,
		address TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, event, channel_id))`,
		"user_id", "event", "channel_id", "address")
	ensureTable("user_events", `CREATE TABLE IF NOT EXISTS user_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		event TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		read INTEGER NOT NULL DEFAULT 0)`,
		"user_id", "event", "title", "body", "created_at", "read")
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_user_events_user ON user_events(user_id, id)`)
	ensureTable("user_ui", `CREATE TABLE IF NOT EXISTS user_ui (
		user_id INTEGER NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, key))`,
		"user_id", "key", "value", "updated_at")
	// v12.6.22: settings an administrator keeps looser than the security level, with a note.
	ensureTable("security_deviations", `CREATE TABLE IF NOT EXISTS security_deviations (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '',
		required TEXT NOT NULL DEFAULT '',
		level TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '',
		user_id INTEGER NOT NULL DEFAULT 0,
		username TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '')`,
		"key", "value", "required", "level", "note", "user_id", "username", "created_at")
	// v12.6.23: roles, teams (also as share members), step-up, break-glass administrators.
	db.Exec(`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE users ADD COLUMN break_glass INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE auth_sessions ADD COLUMN step_up_at TEXT NOT NULL DEFAULT ''`)
	// v13.7: screen lock state per sign-in session and an optional unlock PIN per user.
	db.Exec(`ALTER TABLE auth_sessions ADD COLUMN locked_at TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE auth_sessions ADD COLUMN lock_failures INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE users ADD COLUMN lock_pin_hash TEXT NOT NULL DEFAULT ''`)
	ensureTable("teams", `CREATE TABLE IF NOT EXISTS teams (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		description TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '')`,
		"id", "name", "description", "created_at")
	ensureTable("team_members", `CREATE TABLE IF NOT EXISTS team_members (
		team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		lead INTEGER NOT NULL DEFAULT 0,
		added_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (team_id, user_id))`,
		"team_id", "user_id", "lead", "added_at")
	ensureTable("share_teams", `CREATE TABLE IF NOT EXISTS share_teams (
		share_id INTEGER NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
		team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
		role TEXT NOT NULL DEFAULT 'operator',
		added_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (share_id, team_id))`,
		"share_id", "team_id", "role", "added_at")
	// v12.6.24: OIDC sign-in (identity providers, linked directory identities, SSO team memberships).
	ensureTable("oidc_providers", `CREATE TABLE IF NOT EXISTS oidc_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		issuer TEXT NOT NULL DEFAULT '',
		client_id TEXT NOT NULL DEFAULT '',
		client_secret TEXT NOT NULL DEFAULT '',
		scopes TEXT NOT NULL DEFAULT '',
		redirect_uri TEXT NOT NULL DEFAULT '',
		username_claim TEXT NOT NULL DEFAULT '',
		require_mfa INTEGER NOT NULL DEFAULT 0,
		mfa_values TEXT NOT NULL DEFAULT '',
		jit INTEGER NOT NULL DEFAULT 1,
		rules TEXT NOT NULL DEFAULT '[]',
		enabled INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "name", "issuer", "client_id", "client_secret", "scopes", "redirect_uri", "username_claim", "require_mfa", "mfa_values", "jit", "rules", "enabled", "created_at", "updated_at")
	ensureTable("user_identities", `CREATE TABLE IF NOT EXISTS user_identities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider_id INTEGER NOT NULL REFERENCES oidc_providers(id) ON DELETE CASCADE,
		subject TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		display TEXT NOT NULL DEFAULT '',
		refresh_token TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		last_login_at TEXT NOT NULL DEFAULT '',
		last_check_at TEXT NOT NULL DEFAULT '',
		last_ok_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		revoked INTEGER NOT NULL DEFAULT 0,
		UNIQUE (provider_id, subject))`,
		"id", "user_id", "provider_id", "subject", "email", "display", "refresh_token", "created_at", "last_login_at", "last_check_at", "last_ok_at", "last_error", "revoked")
	db.Exec(`CREATE TABLE IF NOT EXISTS oidc_issuer_checks (issuer TEXT PRIMARY KEY, checked_at TEXT NOT NULL DEFAULT '')`)
	db.Exec(`ALTER TABLE users ADD COLUMN sso_only INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE auth_sessions ADD COLUMN identity_id INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE team_members ADD COLUMN source TEXT NOT NULL DEFAULT ''`)
	// v12.6.25: LDAP / Active Directory sign-in and SCIM provisioning.
	ensureTable("ldap_directories", `CREATE TABLE IF NOT EXISTS ldap_directories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		kind TEXT NOT NULL DEFAULT 'ad',
		url TEXT NOT NULL DEFAULT '',
		ca_pem TEXT NOT NULL DEFAULT '',
		bind_dn TEXT NOT NULL DEFAULT '',
		bind_password TEXT NOT NULL DEFAULT '',
		base_dn TEXT NOT NULL DEFAULT '',
		user_filter TEXT NOT NULL DEFAULT '',
		login_attr TEXT NOT NULL DEFAULT '',
		id_attr TEXT NOT NULL DEFAULT '',
		group_base TEXT NOT NULL DEFAULT '',
		nested INTEGER NOT NULL DEFAULT 1,
		jit INTEGER NOT NULL DEFAULT 1,
		rules TEXT NOT NULL DEFAULT '[]',
		enabled INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		last_sync_at TEXT NOT NULL DEFAULT '',
		last_sync TEXT NOT NULL DEFAULT '')`,
		"id", "name", "kind", "url", "ca_pem", "bind_dn", "bind_password", "base_dn", "user_filter", "login_attr", "id_attr", "group_base", "nested", "jit", "rules", "enabled", "created_at", "updated_at", "last_sync_at", "last_sync")
	ensureTable("ldap_identities", `CREATE TABLE IF NOT EXISTS ldap_identities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		directory_id INTEGER NOT NULL REFERENCES ldap_directories(id) ON DELETE CASCADE,
		object_key TEXT NOT NULL,
		dn TEXT NOT NULL DEFAULT '',
		login TEXT NOT NULL DEFAULT '',
		email TEXT NOT NULL DEFAULT '',
		display TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		last_login_at TEXT NOT NULL DEFAULT '',
		last_check_at TEXT NOT NULL DEFAULT '',
		last_ok_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		revoked INTEGER NOT NULL DEFAULT 0,
		UNIQUE (directory_id, object_key))`,
		"id", "user_id", "directory_id", "object_key", "dn", "login", "email", "display", "created_at", "last_login_at", "last_check_at", "last_ok_at", "last_error", "revoked")
	ensureTable("scim_clients", `CREATE TABLE IF NOT EXISTS scim_clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		token_hash TEXT NOT NULL UNIQUE,
		rules TEXT NOT NULL DEFAULT '[]',
		created_by INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		last_used_at TEXT NOT NULL DEFAULT '',
		last_ip TEXT NOT NULL DEFAULT '',
		revoked_at TEXT NOT NULL DEFAULT '')`,
		"id", "name", "token_hash", "rules", "created_by", "created_at", "last_used_at", "last_ip", "revoked_at")
	ensureTable("scim_users", `CREATE TABLE IF NOT EXISTS scim_users (
		user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		client_id INTEGER NOT NULL REFERENCES scim_clients(id) ON DELETE CASCADE,
		scim_id TEXT NOT NULL UNIQUE,
		external_id TEXT NOT NULL DEFAULT '',
		user_name TEXT NOT NULL DEFAULT '',
		active INTEGER NOT NULL DEFAULT 1,
		deleted INTEGER NOT NULL DEFAULT 0,
		data TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"user_id", "client_id", "scim_id", "external_id", "user_name", "active", "deleted", "data", "created_at", "updated_at")
	ensureTable("scim_groups", `CREATE TABLE IF NOT EXISTS scim_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id INTEGER NOT NULL REFERENCES scim_clients(id) ON DELETE CASCADE,
		scim_id TEXT NOT NULL UNIQUE,
		external_id TEXT NOT NULL DEFAULT '',
		display_name TEXT NOT NULL DEFAULT '',
		team_id INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "client_id", "scim_id", "external_id", "display_name", "team_id", "created_at", "updated_at")
	ensureTable("scim_group_members", `CREATE TABLE IF NOT EXISTS scim_group_members (
		group_id INTEGER NOT NULL REFERENCES scim_groups(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		PRIMARY KEY (group_id, user_id))`,
		"group_id", "user_id")
	// v12.6.26: audit export to a SIEM (cursor = last audit_log id delivered).
	ensureTable("siem_destinations", `CREATE TABLE IF NOT EXISTS siem_destinations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		kind TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT '',
		config TEXT NOT NULL DEFAULT '{}',
		secret TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		cursor INTEGER NOT NULL DEFAULT 0,
		sent INTEGER NOT NULL DEFAULT 0,
		failed INTEGER NOT NULL DEFAULT 0,
		last_ok_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at TEXT NOT NULL DEFAULT '',
		created_by INTEGER,
		created_at TEXT NOT NULL DEFAULT '')`,
		"id", "name", "kind", "url", "config", "secret", "enabled", "cursor", "sent", "failed", "last_ok_at", "last_error", "last_error_at", "created_by", "created_at")
	ensureTable("notify_prefs", `CREATE TABLE IF NOT EXISTS notify_prefs (
		user_id INTEGER PRIMARY KEY,
		quiet_enabled INTEGER NOT NULL DEFAULT 0,
		quiet_start TEXT NOT NULL DEFAULT '22:00',
		quiet_end TEXT NOT NULL DEFAULT '07:00',
		tz TEXT NOT NULL DEFAULT 'UTC',
		lang TEXT NOT NULL DEFAULT 'en')`,
		"user_id", "quiet_enabled", "quiet_start", "quiet_end", "tz", "lang")
	ensureTable("notify_pending", `CREATE TABLE IF NOT EXISTS notify_pending (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		channel_id INTEGER NOT NULL,
		address TEXT NOT NULL DEFAULT '',
		event TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "channel_id", "address", "event", "title", "body", "created_at")
	ensureTable("notify_state", `CREATE TABLE IF NOT EXISTS notify_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '')`,
		"key", "value")

	// v10.9: saved proxies (SOCKS5 / SOCKS4 / HTTP CONNECT / a WRM SOCKS tunnel), shared like credentials
	ensureTable("proxies", `CREATE TABLE IF NOT EXISTS proxies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 'socks5',
		host TEXT NOT NULL DEFAULT '',
		port INTEGER NOT NULL DEFAULT 0,
		username TEXT NOT NULL DEFAULT '',
		password TEXT NOT NULL DEFAULT '',
		credential_id INTEGER DEFAULT NULL,
		remote_dns INTEGER NOT NULL DEFAULT 1,
		tunnel_id INTEGER DEFAULT NULL,
		description TEXT NOT NULL DEFAULT '',
		shared_all INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "owner_id", "name", "kind", "host", "port", "username", "password", "credential_id", "remote_dns", "tunnel_id",
		"description", "shared_all", "created_at", "updated_at")
	ensureTable("proxy_grants", `CREATE TABLE IF NOT EXISTS proxy_grants (
		proxy_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		PRIMARY KEY (proxy_id, user_id))`,
		"proxy_id", "user_id")
	// v10.9.1: imported PuTTY sessions and their proxy, for mRemoteNG connections imported later
	ensureTable("putty_sessions", `CREATE TABLE IF NOT EXISTS putty_sessions (
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL COLLATE NOCASE,
		proxy_id INTEGER DEFAULT NULL,
		PRIMARY KEY (user_id, name))`,
		"user_id", "name", "proxy_id")

	// Safe migrations (columns added over time)
	for _, m := range []string{
		`ALTER TABLE connections ADD COLUMN user_id INTEGER DEFAULT NULL`,
		`ALTER TABLE sessions    ADD COLUMN user_id INTEGER DEFAULT NULL`,
		`ALTER TABLE folders     ADD COLUMN user_id INTEGER DEFAULT NULL`,
		`ALTER TABLE connections ADD COLUMN private_key TEXT DEFAULT ''`,
		`ALTER TABLE connections ADD COLUMN key_path TEXT DEFAULT ''`,
		`ALTER TABLE users       ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE auth_sessions ADD COLUMN last_active_at TEXT DEFAULT ''`,
		`ALTER TABLE sessions    ADD COLUMN layout TEXT NOT NULL DEFAULT ''`,
		// v10: accounts, 2FA, session details
		`ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN totp_pending TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN recovery_codes TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN failed_logins INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN locked_until TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN last_login_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN last_login_ip TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN password_changed_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auth_sessions ADD COLUMN created_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auth_sessions ADD COLUMN ip TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auth_sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auth_sessions ADD COLUMN restricted TEXT NOT NULL DEFAULT ''`,
		// v10: sharing roles, access modes, expiry
		`ALTER TABLE share_links ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'link'`,
		`ALTER TABLE share_links ADD COLUMN guest_role TEXT NOT NULL DEFAULT 'operator'`,
		`ALTER TABLE share_links ADD COLUMN expires_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE share_members ADD COLUMN role TEXT NOT NULL DEFAULT 'operator'`,
		`ALTER TABLE share_members ADD COLUMN added_at TEXT NOT NULL DEFAULT ''`,
		// v10.1: audit entries reference connections and terminal sessions, and are hash-chained
		`ALTER TABLE audit_log ADD COLUMN conn_id INTEGER DEFAULT NULL`,
		`ALTER TABLE audit_log ADD COLUMN session_id INTEGER DEFAULT NULL`,
		`ALTER TABLE audit_log ADD COLUMN prev_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE audit_log ADD COLUMN hash TEXT NOT NULL DEFAULT ''`,
		// v10.2: jump hosts and web interface connections
		`ALTER TABLE connections ADD COLUMN jump_conn_id INTEGER DEFAULT NULL`,
		`ALTER TABLE connections ADD COLUMN web_path TEXT NOT NULL DEFAULT ''`,
		// v10.3: live status monitoring can be turned off per connection
		`ALTER TABLE connections ADD COLUMN monitor INTEGER NOT NULL DEFAULT 1`,
		// v10.4: options of remote desktop connections (RDP / VNC / Telnet), JSON
		`ALTER TABLE connections ADD COLUMN options TEXT NOT NULL DEFAULT ''`,
		// v10.5: logins from the SSH key store and the credentials vault
		`ALTER TABLE connections ADD COLUMN key_id INTEGER DEFAULT NULL`,
		`ALTER TABLE connections ADD COLUMN credential_id INTEGER DEFAULT NULL`,
		// v10.6: tags and the id in a source inventory (NetBox)
		`ALTER TABLE connections ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE connections ADD COLUMN ext_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE connections ADD COLUMN ext_tags TEXT NOT NULL DEFAULT ''`,
		// v10.7: out-of-band management (BMC: Redfish / IPMI), JSON with the password encrypted
		`ALTER TABLE connections ADD COLUMN bmc TEXT NOT NULL DEFAULT ''`,
		// v10.8: notes (runbook) and quick connections (deleted when unused)
		`ALTER TABLE connections ADD COLUMN notes TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE connections ADD COLUMN temp_until TEXT NOT NULL DEFAULT ''`,
		// v10.9: proxies, and default jump host / proxy of a folder
		`ALTER TABLE connections ADD COLUMN proxy_id INTEGER DEFAULT NULL`,
		`ALTER TABLE folders ADD COLUMN parent_id INTEGER DEFAULT NULL`,
		`ALTER TABLE folders ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE folders ADD COLUMN jump_conn_id INTEGER DEFAULT NULL`,
		`ALTER TABLE folders ADD COLUMN proxy_id INTEGER DEFAULT NULL`,
		// v13.14.0: keep this folder's subfolders sorted by name (all levels below it)
		`ALTER TABLE folders ADD COLUMN auto_sort INTEGER NOT NULL DEFAULT 0`,
		// v13.14.0: SSH user certificate saved with a stored key (ssh_cert.go)
		`ALTER TABLE ssh_keys ADD COLUMN certificate TEXT NOT NULL DEFAULT ''`,
		// v10.9.1: the PuTTY session an imported mRemoteNG connection names (PuttySession)
		`ALTER TABLE connections ADD COLUMN putty_session TEXT NOT NULL DEFAULT ''`,
		// v13.1: recording level per user ('' follows the server settings)
		`ALTER TABLE users ADD COLUMN recording_level TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			log.Printf("Migration warning: %v", err)
		}
	}
	db.Exec(`DELETE FROM share_members WHERE rowid NOT IN (SELECT MIN(rowid) FROM share_members GROUP BY share_id, user_id)`)
	for _, q := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_share_members ON share_members(share_id, user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_audit_ts ON audit_log(ts)`,
		`CREATE INDEX IF NOT EXISTS ix_audit_user_ts ON audit_log(user_id, ts)`,
		`CREATE INDEX IF NOT EXISTS ix_audit_conn_ts ON audit_log(conn_id, ts)`,
		`CREATE INDEX IF NOT EXISTS ix_audit_session ON audit_log(session_id)`,
		`CREATE INDEX IF NOT EXISTS ix_tsess_user ON terminal_sessions(user_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS ix_tsess_conn ON terminal_sessions(conn_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS ix_tsess_started ON terminal_sessions(started_at)`,
		`CREATE INDEX IF NOT EXISTS ix_recordings_session ON session_recordings(session_id)`,
		`CREATE INDEX IF NOT EXISTS ix_transfers_ts ON file_transfers(ts)`,
		`CREATE INDEX IF NOT EXISTS ix_transfers_user ON file_transfers(user_id, ts)`,
		`CREATE INDEX IF NOT EXISTS ix_tunnels_conn ON connection_tunnels(conn_id)`,
		`CREATE INDEX IF NOT EXISTS ix_snippets_user ON snippets(user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_bookmarks_user ON bookmarks(user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_ssh_keys_user ON ssh_keys(user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_credentials_owner ON credentials(owner_id)`,
		`CREATE INDEX IF NOT EXISTS ix_conn_credential ON connections(credential_id)`,
		`CREATE INDEX IF NOT EXISTS ix_conn_ext ON connections(user_id, ext_id)`,
		`CREATE INDEX IF NOT EXISTS ix_collab_share ON collab_messages(share_id, id)`,
		`CREATE INDEX IF NOT EXISTS ix_share_items ON share_items(share_id)`,
		`CREATE INDEX IF NOT EXISTS ix_connections_user ON connections(user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_conn_proxy ON connections(proxy_id)`,
		`CREATE INDEX IF NOT EXISTS ix_proxies_owner ON proxies(owner_id)`,
		`CREATE INDEX IF NOT EXISTS ix_notify_pending_user ON notify_pending(user_id, channel_id)`,
		`CREATE INDEX IF NOT EXISTS ix_notify_subs_channel ON notify_subscriptions(channel_id)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("Index warning: %v", err)
		}
	}

	initFileEditsSchema()
	createAppendOnlyTriggers()

	// Session keepalive: update last_active_at for existing sessions that lack it
	db.Exec(`UPDATE auth_sessions SET last_active_at = expires_at WHERE last_active_at = ''`)

	// v11.0: Git workspace (sources, blob cache, catalog, targets, installations)
	initMonitoring()
	initLogSettings()
	initVisualLabels()
	initGitSchema()
	initGitConfigProfiles()

	// v12.0: AI assistant (providers, sessions, transcripts)
	initAISchema()
	initAIChatSchema()
	initAISessionsViewSchema()
	initMCPSchema()

	// v13.0: addons (a folder under <DB>.addons plus its manifest)
	initAddonSchema()

	var nC, nF, nS int
	db.QueryRow("SELECT COUNT(*) FROM connections").Scan(&nC)
	db.QueryRow("SELECT COUNT(*) FROM folders").Scan(&nF)
	db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nS)
	log.Printf("DB ready (%s): %d connections, %d folders, %d sessions", dbPath, nC, nF, nS)
}

func mustExec(q string) {
	if _, err := db.Exec(q); err != nil {
		log.Fatalf("Schema error: %v\n%s", err, q)
	}
}

// ensureTable creates a table introduced in v10. Some earlier WRM builds used the same
// table names with a different layout (e.g. collab_messages keyed by share token). A table
// that lacks a column v10 needs, or has a required column v10 does not fill, is kept under
// a new name (<name>_old_<time>) and created again, so nothing is deleted.
func ensureTable(name, create string, cols ...string) {
	mustExec(create)
	rows, err := db.Query(`SELECT name, "notnull", dflt_value IS NOT NULL, pk FROM pragma_table_info(?)`, name)
	if err != nil {
		log.Printf("Schema check %s: %v", name, err)
		return
	}
	want := map[string]bool{}
	for _, c := range cols {
		want[c] = true
	}
	// Columns the CREATE statement declares itself are never "unknown", even when the
	// caller did not list them (otherwise a NOT NULL column without a default, such as
	// addon_instances.addon_id, would set the table aside on every start).
	for _, c := range createColumns(create) {
		want[c] = true
	}
	have := map[string]bool{}
	var missing, extra []string
	for rows.Next() {
		var col string
		var notNull, hasDefault, pk int
		if rows.Scan(&col, &notNull, &hasDefault, &pk) != nil {
			continue
		}
		col = strings.ToLower(col)
		have[col] = true
		if !want[col] && notNull == 1 && hasDefault == 0 && pk == 0 {
			extra = append(extra, col)
		}
	}
	rows.Close()
	for _, c := range cols {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	reason := "missing " + strings.Join(missing, ", ")
	if len(missing) == 0 {
		reason = "unknown required " + strings.Join(extra, ", ")
	}
	old := name + "_old_" + time.Now().UTC().Format("20060102150405")
	if _, err := db.Exec(`ALTER TABLE "` + name + `" RENAME TO "` + old + `"`); err != nil {
		log.Fatalf("Upgrade of table %s (%s) failed: %v", name, reason, err)
	}
	// The old table's indexes keep their names; drop them so v10 can create its own.
	var idx []string
	if rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name=? AND sql IS NOT NULL`, old); err == nil {
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				idx = append(idx, n)
			}
		}
		rows.Close()
	}
	for _, n := range idx {
		db.Exec(`DROP INDEX IF EXISTS "` + strings.ReplaceAll(n, `"`, `""`) + `"`)
	}
	mustExec(create)
	log.Printf("Upgraded table %s from an earlier version (%s); the old table was kept as %s", name, reason, old)
}

// createColumns returns the lower-cased column names a CREATE TABLE statement declares,
// one column definition per comma-separated item; table constraints are skipped.
func createColumns(create string) []string {
	open, end := strings.Index(create, "("), strings.LastIndex(create, ")")
	if open < 0 || end <= open {
		return nil
	}
	var cols []string
	depth, start := 0, open+1
	body := create[:end]
	for i := open + 1; i <= len(body); i++ {
		if i < len(body) {
			switch body[i] {
			case '(':
				depth++
				continue
			case ')':
				depth--
				continue
			case ',':
				if depth > 0 {
					continue
				}
			default:
				continue
			}
		}
		fields := strings.Fields(body[start:i])
		start = i + 1
		if len(fields) == 0 {
			continue
		}
		name := strings.ToLower(strings.Trim(fields[0], "\"`[]"))
		switch name {
		case "primary", "unique", "foreign", "check", "constraint":
			continue
		}
		cols = append(cols, name)
	}
	return cols
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func decryptConnectionSecrets(c *Connection) {
	c.Password = decryptValue(c.Password)
	c.PrivateKey = decryptValue(c.PrivateKey)
}

func encryptConnectionSecrets(c *Connection) {
	c.Password = encryptValue(c.Password)
	c.PrivateKey = encryptValue(c.PrivateKey)
}

func userOwnsConnection(connectionID, userID int) bool {
	var count int
	db.QueryRow("SELECT COUNT(1) FROM connections WHERE id=? AND user_id=?", connectionID, userID).Scan(&count)
	return count == 1
}

func userOwnsFolder(folderID, userID int) bool {
	var count int
	db.QueryRow("SELECT COUNT(1) FROM folders WHERE id=? AND user_id=?", folderID, userID).Scan(&count)
	return count == 1
}

func userOwnsSession(sessionID, userID int) bool {
	var count int
	db.QueryRow("SELECT COUNT(1) FROM sessions WHERE id=? AND user_id=?", sessionID, userID).Scan(&count)
	return count == 1
}

// ─── CONNECTIONS ─────────────────────────────────────

var validProtocols = map[string]bool{"SSH": true, "SFTP": true, "FTP": true, "FTPS": true, "HTTP": true, "HTTPS": true, "RDP": true, "VNC": true, "TELNET": true, "SERIAL": true}
var validAuthMethods = map[string]bool{"PASSWORD": true, "KEY": true, "KEY_FILE": true, "KEY_AUTO": true, "KEY_REF": true, "CREDENTIAL": true, "BITWARDEN": true}

// serverKeysAllowed: "Key file" and "Auto (~/.ssh)" read private keys of the WRM server
// itself, so only administrators may use them unless the policy allows it.
func serverKeysAllowed(userID int) bool {
	return isAdminUser(userID) || settingBool("allow_server_keys")
}

// usesStoredSecretRef: the connection logs in with a key of the key store or a vault credential.
func usesStoredSecretRef(authMethod string) bool {
	return authMethod == "KEY_REF" || authMethod == "CREDENTIAL" || authMethod == "BITWARDEN"
}

func usesServerKeys(authMethod string) bool {
	return authMethod == "KEY_FILE" || authMethod == "KEY_AUTO"
}

func normalizeConnection(c *Connection) error {
	if err := validateVisualLabel(c.Label); err != nil {
		return err
	}
	c.Name = truncateStr(strings.TrimSpace(c.Name), 120)
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.KeyPath = strings.TrimSpace(c.KeyPath)
	c.Protocol = strings.ToUpper(strings.TrimSpace(c.Protocol))
	c.AuthMethod = strings.ToUpper(strings.TrimSpace(c.AuthMethod))
	if c.Protocol == "" {
		c.Protocol = "SSH"
	}
	if c.AuthMethod == "" {
		c.AuthMethod = "PASSWORD"
	}
	if c.Name == "" || c.Host == "" {
		return fmt.Errorf("Name and host are required")
	}
	if len(c.Host) > 255 || strings.ContainsAny(c.Host, " /\\@") {
		return fmt.Errorf("Invalid host (use host or host:port)")
	}
	if !validProtocols[c.Protocol] {
		return fmt.Errorf("Invalid protocol")
	}
	if !validAuthMethods[c.AuthMethod] {
		return fmt.Errorf("Invalid authentication method")
	}
	// nil = use the folder's default, -1 = none (also when the folder has one)
	for _, p := range []**int{&c.JumpID, &c.ProxyID} {
		if *p != nil && **p == 0 {
			*p = nil
		} else if *p != nil && **p < 0 {
			none := -1
			*p = &none
		}
	}
	if c.Protocol == "SERIAL" {
		// A serial port of the WRM server: the host is the device name (ttyUSB0), options the line settings.
		if !serialNameRe.MatchString(c.Host) {
			return fmt.Errorf("Serial port: enter the device name, e.g. ttyUSB0, ttyS0 or ttyACM0")
		}
		c.AuthMethod, c.Password, c.PrivateKey, c.KeyID, c.CredentialID, c.JumpID, c.ProxyID = "PASSWORD", "", "", nil, nil, nil, nil
		if c.Options != nil {
			opts, err := normalizeSerialOptions(c.Options)
			if err != nil {
				return err
			}
			c.Options = opts
		}
	} else if isDesktopProtocol(c.Protocol) {
		if c.AuthMethod != "PASSWORD" && c.AuthMethod != "CREDENTIAL" {
			return fmt.Errorf("RDP, VNC and Telnet connections use a password or a vault credential")
		}
		if c.Options != nil {
			opts, err := normalizeDesktopOptions(c.Protocol, c.Options)
			if err != nil {
				return err
			}
			c.Options = opts
		}
	} else if c.AuthMethod == "BITWARDEN" {
		c.Options = map[string]string{"bitwarden_item": strings.ToLower(strings.TrimSpace(c.Options["bitwarden_item"]))}
	} else if c.Options != nil {
		c.Options = map[string]string{}
	}
	c.WebPath = strings.TrimSpace(c.WebPath)
	if isWeb(*c) {
		if c.WebPath != "" && !strings.HasPrefix(c.WebPath, "/") {
			c.WebPath = "/" + c.WebPath
		}
		if len(c.WebPath) > 1000 || strings.ContainsAny(c.WebPath, "\r\n\t ") {
			return fmt.Errorf("Invalid web path")
		}
	} else {
		c.WebPath = ""
	}
	return nil
}

// isWeb reports a web interface connection (opened in the browser, optionally through a tunnel).
func isWeb(c Connection) bool {
	p := strings.ToUpper(c.Protocol)
	return p == "HTTP" || p == "HTTPS"
}

func loadUserConnections(userID int) []Connection {
	rows, err := db.Query(`SELECT id,name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,jump_conn_id,web_path,key_id,credential_id,COALESCE(tags,''),proxy_id FROM connections WHERE user_id=? ORDER BY name COLLATE NOCASE, name, id`, userID)
	conns := []Connection{}
	if err != nil {
		return conns
	}
	defer rows.Close()
	defaults := folderDefaultsOf(userID)
	for rows.Next() {
		var c Connection
		var tags string
		rows.Scan(&c.ID, &c.Name, &c.Protocol, &c.Host, &c.Username, &c.AuthMethod, &c.Password, &c.PrivateKey, &c.KeyPath, &c.FolderID, &c.JumpID, &c.WebPath, &c.KeyID, &c.CredentialID, &tags, &c.ProxyID)
		c.Tags = parseTags(tags)
		c.Label = loadVisualLabel("connection", c.ID)
		decryptConnectionSecrets(&c)
		c.UserID = userID
		var fd Folder
		if c.FolderID != nil {
			fd = defaults[*c.FolderID]
		}
		resolveRouteDefaults(&c, fd.JumpID, fd.ProxyID)
		conns = append(conns, c)
	}
	return conns
}

func apiConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		views := []connView{}
		for _, c := range loadUserConnections(userID) {
			views = append(views, c.view())
		}
		jsonOK(w, views)
	case http.MethodPost:
		var c Connection
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if err := normalizeConnection(&c); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if usesServerKeys(c.AuthMethod) && !serverKeysAllowed(userID) {
			jsonError(w, "Key files on the WRM server can only be used by administrators", 403)
			return
		}
		if c.Protocol == "SERIAL" && !serialAllowed(userID) {
			jsonError(w, "Serial ports of the WRM server are not allowed for this account (policy serial_ports)", 403)
			return
		}
		if c.FolderID != nil && !userOwnsFolder(*c.FolderID, userID) {
			c.FolderID = nil
		}
		if err := validateJump(userID, 0, c.JumpID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if err := validateRoute(userID, 0, &c); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if err := checkAuthRefs(&c, userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if usesStoredSecretRef(c.AuthMethod) {
			c.Password, c.PrivateKey = "", ""
		}
		plain := c
		plain.UserID = userID
		applyFolderDefaults(&plain)
		encryptConnectionSecrets(&c)
		res, err := db.Exec(`INSERT INTO connections (name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,jump_conn_id,web_path,key_id,credential_id,tags,proxy_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Protocol, c.Host, c.Username, c.AuthMethod, c.Password, c.PrivateKey, c.KeyPath, c.FolderID, userID, c.JumpID, c.WebPath, c.KeyID, c.CredentialID, tagsString(c.Tags), c.ProxyID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		id, _ := res.LastInsertId()
		plain.ID = int(id)
		saveVisualLabel(db, "connection", plain.ID, userID, c.Label)
		if c.Monitor != nil && !*c.Monitor {
			db.Exec(`UPDATE connections SET monitor=0 WHERE id=?`, id)
		}
		if c.Options != nil {
			db.Exec(`UPDATE connections SET options=? WHERE id=?`, string(jsonMarshal(c.Options)), id)
		}
		if c.BMC != nil {
			if err := saveBMC(plain, c.BMC, userID); err != nil {
				db.Exec(`DELETE FROM connections WHERE id=?`, id)
				jsonError(w, err.Error(), 400)
				return
			}
		}
		if c.Notes != nil {
			if err := validNotes(*c.Notes); err == nil {
				db.Exec(`UPDATE connections SET notes=? WHERE id=?`, *c.Notes, id)
			}
		}
		statusMon.poke()
		auditLogRef(r, userID, usernameOf(userID), "connection.created", c.Name, map[string]string{"host": c.Host, "protocol": c.Protocol, "auth": c.AuthMethod, "route": jumpPath(plain)}, auditRef{ConnID: plain.ID})
		w.WriteHeader(http.StatusCreated)
		if saved, err := loadConnectionRaw(plain.ID); err == nil {
			jsonOK(w, saved.view())
			return
		}
		jsonOK(w, plain.view())
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// /api/connections/{id}            GET · PUT · DELETE
// /api/connections/{id}/duplicate  POST
// /api/connections/{id}/secret     POST (owner, re-authentication; connsecret.go)
// /api/connections/{id}/name-from-hostname  POST
// deleteConnectionRows removes the per-connection rows that are not covered by a
// foreign key. The connection row itself is deleted by the caller after the owner check.
func deleteConnectionRows(connID int) {
	for _, q := range []string{
		`DELETE FROM connection_tunnels WHERE conn_id=?`,
		`DELETE FROM ssh_key_deployments WHERE conn_id=?`,
		`DELETE FROM monitor_samples WHERE conn_id=?`,
		`DELETE FROM monitor_profiles WHERE conn_id=?`,
		`DELETE FROM log_settings WHERE conn_id=?`,
		`DELETE FROM git_server_health WHERE conn_id=?`,
	} {
		db.Exec(q, connID)
	}
}

func apiConnectionByIDHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/connections/"), "/"), "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	if !userOwnsConnection(id, userID) {
		jsonError(w, "Not found", 404)
		return
	}
	cur, err := loadConnectionRaw(id)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		v := cur.view()
		v.Notes = loadNotes(id)
		jsonOK(w, v)
	case action == "" && r.Method == http.MethodPut:
		var in struct {
			Connection
			ClearPassword   bool `json:"clear_password"`
			ClearPrivateKey bool `json:"clear_private_key"`
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(body, &in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		c := in.Connection
		if strings.EqualFold(c.AuthMethod, "BITWARDEN") && c.Options == nil && cur.AuthMethod == "BITWARDEN" {
			c.Options = cur.Options
		}
		// A client that does not send jump_id / proxy_id keeps the stored choices
		// (null would mean "as the folder").
		var keys map[string]json.RawMessage
		json.Unmarshal(body, &keys)
		if _, ok := keys["jump_id"]; !ok {
			c.JumpID = cur.jumpRaw
		}
		if _, ok := keys["proxy_id"]; !ok {
			c.ProxyID = cur.proxyRaw
		}
		if err := normalizeConnection(&c); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if usesServerKeys(c.AuthMethod) && c.AuthMethod != cur.AuthMethod && !serverKeysAllowed(userID) {
			jsonError(w, "Key files on the WRM server can only be used by administrators", 403)
			return
		}
		if c.Protocol == "SERIAL" && (cur.Protocol != "SERIAL" || c.Host != cur.Host) && !serialAllowed(userID) {
			jsonError(w, "Serial ports of the WRM server are not allowed for this account (policy serial_ports)", 403)
			return
		}
		if c.FolderID != nil && !userOwnsFolder(*c.FolderID, userID) {
			c.FolderID = cur.FolderID
		}
		if err := validateJump(userID, id, c.JumpID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if err := validateRoute(userID, id, &c); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if err := checkAuthRefs(&c, userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		// Empty secret fields keep the stored secret (the browser never receives it).
		changed := []string{}
		if c.Password == "" && !in.ClearPassword {
			c.Password = cur.Password
		} else if c.Password != cur.Password {
			changed = append(changed, "password")
		}
		if c.PrivateKey == "" && !in.ClearPrivateKey {
			c.PrivateKey = cur.PrivateKey
		} else if c.PrivateKey != cur.PrivateKey {
			changed = append(changed, "private_key")
		}
		if usesStoredSecretRef(c.AuthMethod) {
			// The key store / vault holds the secret: the connection keeps none of its own.
			c.Password, c.PrivateKey = "", ""
		}
		for f, same := range map[string]bool{"name": c.Name == cur.Name, "host": c.Host == cur.Host, "username": c.Username == cur.Username,
			"protocol": c.Protocol == cur.Protocol, "auth_method": c.AuthMethod == cur.AuthMethod, "key_path": c.KeyPath == cur.KeyPath,
			"jump_host": intPtrEq(c.JumpID, cur.jumpRaw), "web_path": c.WebPath == cur.WebPath, "ssh_key": intPtrEq(c.KeyID, cur.KeyID),
			"credential_ref": intPtrEq(c.CredentialID, cur.CredentialID), "proxy": intPtrEq(c.ProxyID, cur.proxyRaw), "folder": intPtrEq(c.FolderID, cur.FolderID)} {
			if !same {
				changed = append(changed, f)
			}
		}
		encryptConnectionSecrets(&c)
		if _, err := db.Exec(`UPDATE connections SET name=?,protocol=?,host=?,username=?,auth_method=?,password=?,private_key=?,key_path=?,folder_id=?,jump_conn_id=?,web_path=?,key_id=?,credential_id=?,proxy_id=? WHERE id=? AND user_id=?`,
			c.Name, c.Protocol, c.Host, c.Username, c.AuthMethod, c.Password, c.PrivateKey, c.KeyPath, c.FolderID, c.JumpID, c.WebPath, c.KeyID, c.CredentialID, c.ProxyID, id, userID); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		saveVisualLabel(db, "connection", id, userID, c.Label)
		if c.Monitor != nil {
			db.Exec(`UPDATE connections SET monitor=? WHERE id=?`, boolInt(*c.Monitor), id)
		}
		if c.Options != nil {
			db.Exec(`UPDATE connections SET options=? WHERE id=?`, string(jsonMarshal(c.Options)), id)
		}
		if c.Tags != nil {
			db.Exec(`UPDATE connections SET tags=? WHERE id=?`, tagsString(c.Tags), id)
		}
		if c.Notes != nil {
			if err := validNotes(*c.Notes); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			if *c.Notes != loadNotes(id) {
				db.Exec(`UPDATE connections SET notes=? WHERE id=?`, *c.Notes, id)
				changed = append(changed, "notes")
			}
		}
		if c.Temporary != nil && !*c.Temporary {
			// "Save" of a quick connection: it becomes a normal, monitored connection.
			if res, _ := db.Exec(`UPDATE connections SET temp_until='', monitor=1 WHERE id=? AND temp_until<>''`, id); res != nil {
				if n, _ := res.RowsAffected(); n > 0 {
					changed = append(changed, "saved")
					statusMon.poke()
				}
			}
		}
		if c.BMC != nil {
			c.ID, c.UserID = id, userID
			bc := c
			applyFolderDefaults(&bc) // "through the jump host" follows the effective route
			if err := saveBMC(bc, c.BMC, userID); err != nil {
				jsonError(w, "Saved, but the BMC settings were not: "+err.Error(), 400)
				return
			}
		}
		if c.Host != cur.Host || c.Protocol != cur.Protocol || !intPtrEq(c.JumpID, cur.jumpRaw) || !intPtrEq(c.ProxyID, cur.proxyRaw) || !intPtrEq(c.FolderID, cur.FolderID) || c.Monitor != nil {
			statusMon.poke()
		}
		if len(changed) > 0 {
			auditLogRef(r, userID, usernameOf(userID), "connection.updated", c.Name, map[string]interface{}{"id": id, "changed": changed}, auditRef{ConnID: id})
			// Running tunnels keep their SSH connection; restart them so they use the new settings.
			tunnelMgr.restartConn(id)
		}
		jsonOK(w, map[string]bool{"ok": true})
	case action == "" && r.Method == http.MethodDelete:
		killTerminals(func(t *termSession) bool { return t.ConnID == id }, "The connection was deleted")
		tunnelMgr.stopConn(id, "the connection was deleted")
		deleteConnectionRows(id)
		deleteSnippetsForScope("connection", id)
		deleteBookmarksForScope("connection", id)
		db.Exec("UPDATE connections SET jump_conn_id=NULL WHERE jump_conn_id=? AND user_id=?", id, userID)
		db.Exec("UPDATE folders SET jump_conn_id=NULL WHERE jump_conn_id=? AND user_id=?", id, userID)
		db.Exec("DELETE FROM connections WHERE id=? AND user_id=?", id, userID)
		deleteGitInstallsOf(userID, id)
		auditLog(r, userID, "connection.deleted", cur.Name, map[string]interface{}{"id": id, "host": cur.Host})
		jsonOK(w, map[string]bool{"ok": true})
	case action == "tunnels":
		connectionTunnelsHandler(w, r, userID, cur)
	case action == "open-web" && r.Method == http.MethodPost:
		openWebHandler(w, r, userID, cur)
	case action == "authorized-keys":
		authorizedKeysHandler(w, r, userID, id)
	case action == "bmc":
		bmcHandler(w, r, userID, cur)
	case action == "secret":
		connectionSecretHandler(w, r, userID, cur)
	case action == "name-from-hostname":
		connectionNameFromHostnameHandler(w, r, userID, id)
	case action == "duplicate" && r.Method == http.MethodPost:
		res, err := db.Exec(`INSERT INTO connections (name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,jump_conn_id,web_path,monitor,options,key_id,credential_id,tags,bmc,notes,proxy_id)
			SELECT name || ' (copy)',protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,jump_conn_id,web_path,monitor,options,key_id,credential_id,tags,bmc,notes,proxy_id FROM connections WHERE id=? AND user_id=?`, id, userID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		nid, _ := res.LastInsertId()
		copyTunnelDefs(id, int(nid))
		saveVisualLabel(db, "connection", int(nid), userID, loadVisualLabel("connection", id))
		copySnippetsForConnection(id, int(nid), userID)
		copyBookmarksForConnection(id, int(nid), userID)
		dup, _ := loadConnectionRaw(int(nid))
		auditLog(r, userID, "connection.created", dup.Name, map[string]interface{}{"duplicate_of": id})
		jsonOK(w, dup.view())
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func apiConnectionsBulkHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var payload struct {
		Action   string   `json:"action"`
		IDs      []int    `json:"ids"`
		FolderID *int     `json:"folder_id"`
		Add      []string `json:"add"`    // tag
		Remove   []string `json:"remove"` // tag
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if len(payload.IDs) == 0 || len(payload.IDs) > 5000 {
		jsonError(w, "No IDs provided", 400)
		return
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(payload.IDs)), ",")
	args := make([]interface{}, len(payload.IDs))
	for i, id := range payload.IDs {
		args[i] = id
	}
	switch strings.ToLower(payload.Action) {
	case "delete":
		ids := map[int]bool{}
		for _, id := range payload.IDs {
			if userOwnsConnection(id, userID) {
				ids[id] = true
			}
		}
		killTerminals(func(t *termSession) bool { return ids[t.ConnID] }, "The connection was deleted")
		for id := range ids {
			tunnelMgr.stopConn(id, "the connection was deleted")
			deleteConnectionRows(id)
			deleteSnippetsForScope("connection", id)
			deleteBookmarksForScope("connection", id)
			db.Exec("UPDATE connections SET jump_conn_id=NULL WHERE jump_conn_id=? AND user_id=?", id, userID)
			db.Exec("UPDATE folders SET jump_conn_id=NULL WHERE jump_conn_id=? AND user_id=?", id, userID)
			deleteGitInstallsOf(userID, id)
		}
		db.Exec("DELETE FROM connections WHERE id IN ("+ph+") AND user_id=?", append(args, userID)...)
		auditLog(r, userID, "connection.deleted", "", map[string]interface{}{"ids": payload.IDs})
	case "move":
		if payload.FolderID != nil && !userOwnsFolder(*payload.FolderID, userID) {
			jsonError(w, "Folder not found", 404)
			return
		}
		// The folder's defaults (jump host, proxy) may change the route of the moved connections:
		// remember where they were, check the new routes, and put them back when one breaks.
		oldFolder := map[int]*int{}
		if rows, err := db.Query("SELECT id, folder_id FROM connections WHERE id IN ("+ph+") AND user_id=?", append(args, userID)...); err == nil {
			for rows.Next() {
				var id int
				var f *int
				rows.Scan(&id, &f)
				if !intPtrEq(f, payload.FolderID) {
					oldFolder[id] = f
				}
			}
			rows.Close()
		}
		db.Exec("UPDATE connections SET folder_id=? WHERE id IN ("+ph+") AND user_id=?", append([]interface{}{payload.FolderID}, append(args, userID)...)...)
		moved := make([]int, 0, len(oldFolder))
		for id := range oldFolder {
			moved = append(moved, id)
		}
		if err := validateRoutesOf(userID, moved); err != nil {
			for id, f := range oldFolder {
				db.Exec("UPDATE connections SET folder_id=? WHERE id=? AND user_id=?", f, id, userID)
			}
			jsonError(w, "Not moved: the folder's defaults would break connection "+err.Error(), 400)
			return
		}
		for _, id := range moved {
			tunnelMgr.restartConn(id)
		}
		statusMon.poke()
	case "tag":
		n := bulkTag(userID, payload.IDs, payload.Add, payload.Remove)
		auditLog(r, userID, "connection.tagged", "", map[string]interface{}{"ids": payload.IDs, "add": normalizeTags(payload.Add), "remove": normalizeTags(payload.Remove), "changed": n})
		jsonOK(w, map[string]interface{}{"ok": true, "changed": n})
		return
	default:
		jsonError(w, "Unsupported action", 400)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// ─── FOLDERS ─────────────────────────────────────────

func apiFoldersHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, loadFolders(userID))
	case http.MethodPost:
		folderHierarchyMu.Lock()
		defer folderHierarchyMu.Unlock()
		var f Folder
		if json.NewDecoder(r.Body).Decode(&f) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		f.Name = truncateStr(strings.TrimSpace(f.Name), 120)
		if f.Name == "" {
			jsonError(w, "Name required", 400)
			return
		}
		if err := validateFolderDefaults(userID, &f); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		siblings := []Folder{}
		for _, x := range loadFolders(userID) {
			if intPtrEq(x.ParentID, f.ParentID) {
				siblings = append(siblings, x)
			}
		}
		// Folder siblings keep the user's manual order: a new folder is appended and
		// can then be moved. Connections are the list that always sorts by name.
		index := len(siblings)
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		res, err := tx.Exec("INSERT INTO folders (name,user_id,jump_conn_id,proxy_id,parent_id,sort_order) VALUES (?,?,?,?,?,?)", f.Name, userID, f.JumpID, f.ProxyID, f.ParentID, index+1)
		if err != nil {
			tx.Rollback()
			jsonError(w, err.Error(), 500)
			return
		}
		id, _ := res.LastInsertId()
		f.ID = int(id)
		order := make([]int, 0, len(siblings)+1)
		for i, x := range siblings {
			if i == index {
				order = append(order, f.ID)
			}
			order = append(order, x.ID)
		}
		if index >= len(siblings) {
			order = append(order, f.ID)
		}
		for i, fid := range order {
			if _, err := tx.Exec("UPDATE folders SET sort_order=? WHERE id=? AND user_id=?", i+1, fid, userID); err != nil {
				tx.Rollback()
				jsonError(w, err.Error(), 500)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		saveVisualLabel(db, "folder", f.ID, userID, f.Label)
		autoSortFolders(userID)
		if f.JumpID != nil || f.ProxyID != nil {
			auditLog(r, userID, "folder.created", f.Name, map[string]interface{}{"folder_id": f.ID, "default_jump": f.JumpID, "default_proxy": f.ProxyID})
		}
		w.WriteHeader(http.StatusCreated)
		for _, x := range loadFolders(userID) {
			if x.ID == f.ID {
				jsonOK(w, x)
				return
			}
		}
		jsonOK(w, f)
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func apiFolderByIDHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	folderHierarchyMu.Lock()
	defer folderHierarchyMu.Unlock()
	idStr := strings.TrimPrefix(r.URL.Path, "/api/folders/")
	id, _ := strconv.Atoi(strings.TrimSpace(idStr))
	if !userOwnsFolder(id, userID) {
		jsonError(w, "Not found", 404)
		return
	}
	if r.Method == http.MethodPut {
		updateFolder(w, r, userID, id)
		return
	}
	if r.Method != http.MethodDelete {
		jsonError(w, "Method not allowed", 405)
		return
	}
	folderName := ""
	for _, f := range loadFolders(userID) {
		if f.ID == id {
			folderName = f.Name
			break
		}
	}
	folderIDs, connIDs, err := deleteFolderTree(userID, id)
	if err != nil {
		if err.Error() == "folder not found" {
			jsonError(w, "Not found", 404)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	// Running terminals and tunnels would otherwise keep a connection that no longer exists.
	if len(connIDs) > 0 {
		set := make(map[int]bool, len(connIDs))
		for _, cid := range connIDs {
			set[cid] = true
			tunnelMgr.stopConn(cid, "the connection was deleted with its folder")
		}
		killTerminals(func(t *termSession) bool { return set[t.ConnID] }, "The connection was deleted with its folder")
	}
	revalidateFolderShares(userID)
	auditLog(r, userID, "folder.deleted", folderName, map[string]interface{}{"folder_id": id, "folders": len(folderIDs), "connections": len(connIDs), "recursive": true})
	jsonOK(w, map[string]interface{}{"ok": true, "folders": len(folderIDs), "connections": len(connIDs)})
}

// ─── SESSIONS ─────────────────────────────────────────

func loadSessionRows(rows *sql.Rows) []Session {
	sessions := make([]Session, 0)
	for rows.Next() {
		var s Session
		var locked int
		var p1name, p2name *string
		if err := rows.Scan(&s.ID, &s.Name, &s.CreatedAt, &locked,
			&s.Pane1ConnID, &s.Pane1Mode, &s.Pane2ConnID, &s.Pane2Mode,
			&p1name, &p2name, &s.Layout); err != nil {
			continue
		}
		s.Locked = locked == 1
		if p1name != nil {
			s.Pane1ConnName = *p1name
		}
		if p2name != nil {
			s.Pane2ConnName = *p2name
		}
		sessions = append(sessions, s)
	}
	return sessions
}

func apiSessionsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`
                        SELECT s.id, s.name, s.created_at, s.locked,
                               s.pane1_conn_id, s.pane1_mode, s.pane2_conn_id, s.pane2_mode,
                               c1.name, c2.name, s.layout
                        FROM sessions s
                        LEFT JOIN connections c1 ON s.pane1_conn_id = c1.id
                        LEFT JOIN connections c2 ON s.pane2_conn_id = c2.id
                        WHERE s.user_id=? ORDER BY s.created_at DESC`, userID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		jsonOK(w, loadSessionRows(rows))
	case http.MethodPost:
		var s Session
		json.NewDecoder(r.Body).Decode(&s)
		if s.Name == "" {
			s.Name = time.Now().Format("Jan 2, 2006 15:04")
		}
		if s.CreatedAt == "" {
			s.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
		}
		locked := 0
		if s.Locked {
			locked = 1
		}
		if len(s.Layout) > maxSessionLayout {
			jsonError(w, "Session layout too large", 400)
			return
		}
		res, err := db.Exec(`INSERT INTO sessions (name,created_at,locked,pane1_conn_id,pane1_mode,pane2_conn_id,pane2_mode,user_id,layout) VALUES (?,?,?,?,?,?,?,?,?)`,
			s.Name, s.CreatedAt, locked, s.Pane1ConnID, s.Pane1Mode, s.Pane2ConnID, s.Pane2Mode, userID, s.Layout)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		id, _ := res.LastInsertId()
		s.ID = int(id)
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, s)
		broadcastSessionUpdate(userID)
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func apiSessionByIDHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	id, err := strconv.Atoi(strings.TrimSpace(idStr))
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	if !userOwnsSession(id, userID) {
		jsonError(w, "Not found", 404)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var s struct {
			Session
			Layout *string `json:"layout"` // nil = keep the stored layout
		}
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		var curLocked int
		var curLayout string
		db.QueryRow("SELECT locked, layout FROM sessions WHERE id=?", id).Scan(&curLocked, &curLayout)
		layout := curLayout
		if s.Layout != nil {
			if curLocked == 1 && s.Locked && *s.Layout != curLayout {
				jsonError(w, "Session is locked — unlock it before overwriting its windows", 403)
				return
			}
			if len(*s.Layout) > maxSessionLayout {
				jsonError(w, "Session layout too large", 400)
				return
			}
			layout = *s.Layout
		}
		if strings.TrimSpace(s.Name) == "" {
			jsonError(w, "Name required", 400)
			return
		}
		locked := 0
		if s.Locked {
			locked = 1
		}
		if _, err := db.Exec(`UPDATE sessions SET name=?,locked=?,pane1_conn_id=?,pane1_mode=?,pane2_conn_id=?,pane2_mode=?,layout=? WHERE id=?`,
			s.Name, locked, s.Pane1ConnID, s.Pane1Mode, s.Pane2ConnID, s.Pane2Mode, layout, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusOK)
		broadcastSessionUpdate(userID)
	case http.MethodDelete:
		var locked int
		db.QueryRow("SELECT locked FROM sessions WHERE id=?", id).Scan(&locked)
		if locked == 1 && r.URL.Query().Get("force") != "true" {
			jsonError(w, "Session is locked. Add ?force=true to delete.", 403)
			return
		}
		db.Exec("DELETE FROM sessions WHERE id=? AND user_id=?", id, userID)
		w.WriteHeader(http.StatusOK)
		broadcastSessionUpdate(userID)
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// ─── EXPORT/IMPORT ───────────────────────────────────

// GET  /api/config/export                → folders + connections WITHOUT passwords/keys
// POST /api/config/export {password}     → including passwords/keys (re-authentication)
func apiExportHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	withSecrets := false
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		var p struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		u, err := loadUser("id=?", userID)
		if err != nil {
			jsonError(w, "Unauthorized", 401)
			return
		}
		switch checkPasswordLimited(r, u, p.Password) {
		case pwLocked:
			jsonError(w, "Too many attempts. Try again later.", 429)
			return
		case pwWrong:
			auditLog(r, userID, "config.export_denied", "", map[string]string{"reason": "wrong password"})
			jsonError(w, "Wrong password", 403)
			return
		}
		if !u.IsAdmin && !settingBool("allow_secret_export") {
			jsonError(w, "Exporting passwords and keys is disabled by the administrator", 403)
			return
		}
		if !requireStepUp(w, r, "config_export_secrets") {
			return
		}
		withSecrets = true
	default:
		jsonError(w, "Method not allowed", 405)
		return
	}
	folders := loadFolders(userID)
	for i := range folders {
		// The default proxy is referenced by name; the jump host by the exported connection id.
		folders[i].ProxyRef, folders[i].ProxyID, folders[i].JumpName, folders[i].ProxyName = folders[i].ProxyName, nil, "", ""
	}
	all := loadUserConnections(userID)
	conns := all[:0]
	for _, c := range all {
		var temp string
		db.QueryRow(`SELECT COALESCE(temp_until,'') FROM connections WHERE id=?`, c.ID).Scan(&temp)
		if temp == "" { // quick connections are not exported
			conns = append(conns, c)
		}
	}
	for i := range conns {
		if n := loadNotes(conns[i].ID); n != "" {
			conns[i].Notes = &n
		}
		if !withSecrets {
			conns[i].Password, conns[i].PrivateKey = "", ""
		}
		// Keys and credentials stay in the key store / vault; the export names them.
		if conns[i].KeyID != nil {
			db.QueryRow(`SELECT name FROM ssh_keys WHERE id=? AND user_id=?`, *conns[i].KeyID, userID).Scan(&conns[i].KeyRef)
		}
		if conns[i].CredentialID != nil {
			db.QueryRow(`SELECT name FROM credentials WHERE id=?`, *conns[i].CredentialID).Scan(&conns[i].CredentialRef)
		}
		// The stored choices, not the folder defaults: null = as the folder, -1 = none.
		conns[i].JumpID, conns[i].ProxyID = conns[i].jumpRaw, conns[i].proxyRaw
		if conns[i].ProxyID != nil && *conns[i].ProxyID > 0 {
			db.QueryRow(`SELECT name FROM proxies WHERE id=?`, *conns[i].ProxyID).Scan(&conns[i].ProxyRef)
			conns[i].ProxyID = nil
		}
		conns[i].KeyID, conns[i].CredentialID = nil, nil
		if isDesktopProtocol(conns[i].Protocol) || conns[i].Protocol == "SERIAL" || conns[i].AuthMethod == "BITWARDEN" {
			conns[i].Options = loadDesktopOptions(conns[i].ID)
		}
		if b, ok := loadBMC(conns[i].ID); ok {
			if !withSecrets {
				b.Password = ""
			}
			b.HasPassword, b.CredentialID = false, nil // credentials stay in the vault
			conns[i].BMC = &b
		}
	}
	auditLog(r, userID, "config.exported", "", map[string]interface{}{"connections": len(conns), "with_secrets": withSecrets})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", attachmentHeader("wrm-config-"+AppVersion+".json"))
	json.NewEncoder(w).Encode(map[string]interface{}{"version": AppVersion, "with_secrets": withSecrets, "folders": folders, "connections": conns,
		"tunnels": loadTunnelDefs("user_id=?", userID), "snippets": loadSnippets("user_id=?", userID), "proxies": exportProxies(userID, withSecrets),
		"bookmarks": loadBookmarks("user_id=?", userID)})
}

func apiImportHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var payload struct {
		Folders     []Folder      `json:"folders"`
		Connections []Connection  `json:"connections"`
		Tunnels     []tunnelDef   `json:"tunnels"`
		Snippets    []Snippet     `json:"snippets"`
		Proxies     []proxyExport `json:"proxies"`
		Bookmarks   []Bookmark    `json:"bookmarks"`
	}
	if !decodeImportJSON(w, r, &payload) {
		return
	}
	folderHierarchyMu.Lock()
	defer folderHierarchyMu.Unlock()
	order, err := folderImportOrder(payload.Folders)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	// Proxies first (by name: an existing one of the same name is used), so connections can link to them.
	proxiesImported := importProxies(r, userID, payload.Proxies, false, nil)
	allowServerKeys := serverKeysAllowed(userID)
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	folderIDs := map[string]*int{}
	for _, index := range order {
		f := payload.Folders[index]
		f.Name = truncateStr(strings.TrimSpace(f.Name), 120)
		if f.Name == "" {
			continue
		}
		var parent *int
		if f.ParentID != nil {
			parent = folderIDs[fmt.Sprintf("id:%d", *f.ParentID)]
		}
		var id int
		if err := tx.QueryRow(`SELECT id FROM folders WHERE name=? AND user_id=? AND parent_id IS ? ORDER BY id LIMIT 1`, f.Name, userID, parent).Scan(&id); err != nil {
			res, err := tx.Exec(`INSERT INTO folders (name, user_id, parent_id, sort_order) VALUES (?,?,?,?)`, f.Name, userID, parent, f.SortOrder)
			if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
			n, _ := res.LastInsertId()
			id = int(n)
		}
		if err := saveVisualLabel(tx, "folder", id, userID, f.Label); err != nil {
			tx.Rollback()
			jsonError(w, err.Error(), 400)
			return
		}
		folderIDs[folderImportKey(f)] = &id
	}
	imported, skipped := 0, 0
	newIDs := map[int]int{} // exported connection id → new id
	type jumpLink struct{ id, oldJump int }
	var jumps []jumpLink
	type proxyLink struct {
		id  int
		ref string
	}
	var proxyLinks []proxyLink
	for _, c := range payload.Connections {
		oldID, oldJump := c.ID, 0
		if c.JumpID != nil {
			oldJump = *c.JumpID
		}
		noProxy := c.ProxyID != nil && *c.ProxyID < 0 // "none" (also when the folder has a default)
		c.JumpID, c.ProxyID = nil, nil
		// Keys and credentials are referenced by name: use the importing user's ones.
		c.KeyID, c.CredentialID = nil, nil
		switch strings.ToUpper(c.AuthMethod) {
		case "KEY_REF":
			var kid int
			if c.KeyRef != "" && db.QueryRow(`SELECT id FROM ssh_keys WHERE user_id=? AND name=? AND private_key<>'' ORDER BY id LIMIT 1`, userID, c.KeyRef).Scan(&kid) == nil {
				c.KeyID = &kid
			} else {
				c.AuthMethod = "PASSWORD"
			}
		case "CREDENTIAL":
			c.AuthMethod = "PASSWORD"
			for _, cr := range credentialsForUser(userID) {
				if cr.Name == c.CredentialRef && c.CredentialRef != "" && hostAllowed(cr.Hosts, c.Host) {
					cid := cr.ID
					c.AuthMethod, c.CredentialID = "CREDENTIAL", &cid
					break
				}
			}
		}
		if normalizeConnection(&c) != nil || (usesServerKeys(c.AuthMethod) && !allowServerKeys) {
			skipped++
			continue
		}
		if usesStoredSecretRef(c.AuthMethod) {
			c.Password, c.PrivateKey = "", ""
		}
		var folderID *int
		if c.FolderID != nil {
			for _, f := range payload.Folders {
				if f.ID == *c.FolderID && folderIDs[folderImportKey(f)] != nil {
					folderID = folderIDs[folderImportKey(f)]
					break
				}
			}
		}
		encryptConnectionSecrets(&c)
		if res, err := tx.Exec(`INSERT INTO connections (name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,web_path,key_id,credential_id,tags) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Protocol, c.Host, c.Username, c.AuthMethod, c.Password, c.PrivateKey, c.KeyPath, folderID, userID, c.WebPath, c.KeyID, c.CredentialID, tagsString(c.Tags)); err == nil {
			imported++
			nid, _ := res.LastInsertId()
			if oldID > 0 {
				newIDs[oldID] = int(nid)
			}
			if err := saveVisualLabel(tx, "connection", int(nid), userID, c.Label); err != nil {
				tx.Rollback()
				jsonError(w, err.Error(), 400)
				return
			}
			if len(c.Options) > 0 {
				tx.Exec(`UPDATE connections SET options=? WHERE id=?`, string(jsonMarshal(c.Options)), nid)
			}
			if c.Notes != nil && *c.Notes != "" && validNotes(*c.Notes) == nil {
				tx.Exec(`UPDATE connections SET notes=? WHERE id=?`, *c.Notes, nid)
			}
			if c.BMC != nil && c.BMC.Type != "" {
				b := *c.BMC
				b.CredentialID, b.ViaJump = nil, false
				b.Type = strings.ToLower(b.Type)
				if (b.Type == "redfish" || b.Type == "ipmi") && bmcHostRe.MatchString(b.Host) {
					b.Password, b.HasPassword = encryptValue(b.Password), false
					tx.Exec(`UPDATE connections SET bmc=? WHERE id=?`, string(jsonMarshal(b)), nid)
				}
			}
			if oldJump > 0 {
				jumps = append(jumps, jumpLink{int(nid), oldJump})
			} else if oldJump < 0 {
				tx.Exec(`UPDATE connections SET jump_conn_id=-1 WHERE id=?`, nid)
			}
			if noProxy {
				tx.Exec(`UPDATE connections SET proxy_id=-1 WHERE id=?`, nid)
			}
			if c.ProxyRef != "" {
				proxyLinks = append(proxyLinks, proxyLink{int(nid), c.ProxyRef})
			}
		}
	}
	for _, j := range jumps {
		if nj, ok := newIDs[j.oldJump]; ok {
			tx.Exec(`UPDATE connections SET jump_conn_id=? WHERE id=?`, nj, j.id)
		}
	}
	// Folder default jump hosts point to imported connections.
	for _, f := range payload.Folders {
		if nf := folderIDs[folderImportKey(f)]; nf != nil && f.JumpID != nil {
			if nj, ok := newIDs[*f.JumpID]; ok {
				tx.Exec(`UPDATE folders SET jump_conn_id=? WHERE id=? AND jump_conn_id IS NULL`, nj, *nf)
			}
		}
	}
	tunnelsImported := 0
	newTunnels := map[int]int{}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range payload.Tunnels {
		nc, ok := newIDs[d.ConnID]
		oldTunnel := d.ID
		if !ok || validateTunnelDef(&d, userID) != nil {
			continue
		}
		if res, err := tx.Exec(`INSERT INTO connection_tunnels (conn_id, user_id, name, kind, bind_host, bind_port, target_host, target_port, open_scheme, open_path, start_mode, sort, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, nc, userID, d.Name, d.Kind, d.BindHost, d.BindPort, d.TargetHost, d.TargetPort, d.OpenScheme, d.OpenPath, d.StartMode, d.Sort, now); err == nil {
			tunnelsImported++
			if nt, _ := res.LastInsertId(); oldTunnel > 0 {
				newTunnels[oldTunnel] = int(nt)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// "WRM SOCKS tunnel" proxies need the imported tunnels; then the connections are linked.
	proxiesImported += importProxies(r, userID, payload.Proxies, true, newTunnels)
	for _, f := range payload.Folders {
		if nf := folderIDs[folderImportKey(f)]; nf != nil && f.ProxyRef != "" {
			if pid := proxyByName(userID, f.ProxyRef); pid > 0 {
				db.Exec(`UPDATE folders SET proxy_id=? WHERE id=? AND proxy_id IS NULL`, pid, *nf)
			}
		}
	}
	for _, pl := range proxyLinks {
		if pid := proxyByName(userID, pl.ref); pid > 0 {
			c, _ := loadConnectionRaw(pl.id)
			c.ProxyID = &pid
			if validateProxyChoice(userID, pl.id, &c) == nil {
				db.Exec(`UPDATE connections SET proxy_id=? WHERE id=?`, pid, pl.id)
			}
		}
	}
	// Snippets are added after the commit, so their folder/connection checks see the new rows.
	oldFolders := map[int]int{}
	for _, f := range payload.Folders {
		if nf := folderIDs[folderImportKey(f)]; nf != nil && f.ID > 0 {
			oldFolders[f.ID] = *nf
		}
	}
	snippetsImported := 0
	existing := map[string]bool{}
	for _, s := range loadSnippets("user_id=?", userID) {
		existing[s.Name+"\x00"+s.Command] = true
	}
	for _, sn := range payload.Snippets {
		sn.Shared = false
		switch sn.Scope {
		case "folder":
			sn.ScopeID = oldFolders[sn.ScopeID]
		case "connection":
			sn.ScopeID = newIDs[sn.ScopeID]
		}
		if validateSnippet(&sn, userID) != nil || existing[sn.Name+"\x00"+sn.Command] {
			continue
		}
		existing[sn.Name+"\x00"+sn.Command] = true
		if _, err := db.Exec(`INSERT INTO snippets (user_id, name, command, description, grp, scope, scope_id, auto_run, shared, sort, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,0,?,?,?)`,
			userID, sn.Name, sn.Command, sn.Description, sn.Group, sn.Scope, sn.ScopeID, boolInt(sn.AutoRun), sn.Sort, now, now); err == nil {
			snippetsImported++
		}
	}
	if snippetsImported > 0 {
		notifySnippetsChanged(userID, false)
	}
	bookmarksImported := importBookmarks(userID, payload.Bookmarks, oldFolders, newIDs)
	auditLog(r, userID, "config.imported", "", map[string]int{"connections": imported, "skipped": skipped, "tunnels": tunnelsImported, "snippets": snippetsImported, "proxies": proxiesImported, "bookmarks": bookmarksImported})
	jsonOK(w, map[string]int{"imported": imported, "skipped": skipped, "tunnels": tunnelsImported, "snippets": snippetsImported, "proxies": proxiesImported, "bookmarks": bookmarksImported})
}

// ─── EVENTS WS ───────────────────────────────────────

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := currentUserID(r)
	if err != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ws.SetReadLimit(4096)
	ch := hub.subscribe(userID)
	defer hub.unsubscribe(ch)
	// Reader: detects closed/dead clients.
	gone := make(chan struct{})
	ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(90 * time.Second)); return nil })
	go func() {
		defer close(gone)
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}()
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-gone:
			return
		case <-t.C:
			if ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)) != nil {
				return
			}
			// Signed out or disabled in the meantime: close the event stream.
			if _, err := currentUserID(r); err != nil {
				return
			}
		case msg := <-ch:
			ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if ws.WriteMessage(websocket.TextMessage, msg) != nil {
				return
			}
		}
	}
}

// ─── SSH KEY HELPERS ──────────────────────────────────

func parsePrivateKeyBytes(data []byte) (ssh.Signer, error) {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "ssh-") || strings.HasPrefix(trimmed, "ecdsa-") {
		return nil, fmt.Errorf("this is a public key; use the private key file")
	}
	s, err := ssh.ParsePrivateKey([]byte(trimmed))
	if err != nil {
		return nil, fmt.Errorf("key parse error: %v", err)
	}
	return s, nil
}

func findPrivateForPub(pubPath string) ([]byte, string, error) {
	cands := []string{}
	if strings.HasSuffix(pubPath, ".pub") {
		cands = append(cands, strings.TrimSuffix(pubPath, ".pub"))
	}
	home, _ := os.UserHomeDir()
	for _, n := range []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"} {
		cands = append(cands, filepath.Join(home, ".ssh", n))
	}
	for _, p := range cands {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "PRIVATE KEY") {
			return data, p, nil
		}
	}
	return nil, "", fmt.Errorf("private key not found")
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return filepath.FromSlash(p)
}

func resolveKeyMaterial(c Connection) ([]byte, error) {
	kd, _, err := resolveKeyMaterialPath(c)
	return kd, err
}

// resolveKeyMaterialPath returns the private key of a connection and, for key files on the
// WRM server, the file it was read from (its certificate lies next to it).
func resolveKeyMaterialPath(c Connection) ([]byte, string, error) {
	switch c.AuthMethod {
	case "KEY":
		d, _ := splitKeyAndCert(strings.TrimSpace(c.PrivateKey))
		if d == "" {
			return nil, "", fmt.Errorf("private key not provided")
		}
		return []byte(d), "", nil
	case "KEY_FILE":
		p := expandPath(strings.TrimSpace(c.KeyPath))
		if p == "" {
			return nil, "", fmt.Errorf("key path not provided")
		}
		p = strings.TrimSuffix(p, "-cert.pub") // the certificate of a key: use the key next to it
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read '%s': %v", p, err)
		}
		t := strings.TrimSpace(string(data))
		if strings.HasSuffix(strings.ToLower(p), ".pub") || strings.HasPrefix(t, "ssh-") || strings.HasPrefix(t, "ecdsa-") {
			priv, found, err := findPrivateForPub(p)
			if err != nil {
				return nil, "", fmt.Errorf("found .pub but no private key: %v", err)
			}
			log.Printf("Using key: %s", found)
			return priv, found, nil
		}
		return data, p, nil
	case "KEY_AUTO":
		home, _ := os.UserHomeDir()
		var first []byte
		firstPath := ""
		for _, n := range []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"} {
			p := filepath.Join(home, ".ssh", n)
			data, err := os.ReadFile(p)
			if err != nil || !strings.Contains(string(data), "PRIVATE KEY") {
				continue
			}
			// a key with a valid certificate next to it (opkssh login) comes first
			if cert, err := readCertFile(certFileFor(p)); err == nil && cert != nil && !certExpired(cert, time.Now()) {
				return data, p, nil
			}
			if first == nil {
				first, firstPath = data, p
			}
		}
		if first != nil {
			return first, firstPath, nil
		}
		return nil, "", fmt.Errorf("no key found in ~/.ssh/")
	}
	return nil, "", fmt.Errorf("unknown auth method: %s", c.AuthMethod)
}

func buildAuthMethods(c Connection) ([]ssh.AuthMethod, error) {
	if usesStoredSecretRef(c.AuthMethod) && c.authErr == "" {
		resolveConnectionAuth(&c)
	}
	if c.authErr != "" {
		return nil, errors.New(c.authErr)
	}
	if c.usedKeyID > 0 {
		touchKeyUsed(c.usedKeyID)
	}
	if c.AuthMethod == "PASSWORD" || c.AuthMethod == "" {
		pw := c.Password
		// Many servers only offer keyboard-interactive; answer password prompts with it.
		ki := ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = pw
			}
			return answers, nil
		})
		return []ssh.AuthMethod{ssh.Password(pw), ki}, nil
	}
	if usesServerKeys(c.AuthMethod) && c.UserID > 0 && !serverKeysAllowed(c.UserID) {
		return nil, fmt.Errorf("key files on the WRM server are disabled by the administrator for this account")
	}
	kd, keyPath, err := resolveKeyMaterialPath(c)
	if err != nil {
		return nil, err
	}
	sg, err := parsePrivateKeyBytes(kd)
	if err != nil {
		return nil, err
	}
	signers, err := certSigners(c, keyPath, sg)
	if err != nil {
		return nil, err
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil
}

func ensurePort(host, protocol string) string {
	if strings.Contains(host, ":") {
		return host
	}
	switch strings.ToUpper(protocol) {
	case "FTP", "FTPS":
		return host + ":21"
	case "HTTP":
		return host + ":80"
	case "HTTPS":
		return host + ":443"
	case "RDP":
		return host + ":3389"
	case "VNC":
		return host + ":5900"
	case "TELNET":
		return host + ":23"
	case "SERIAL":
		return host
	}
	return host + ":22"
}

// getSSHClient connects to c (through its jump hosts, if any).
func getSSHClient(c Connection) (*ssh.Client, error) {
	return dialSSH(c, nil)
}

// ─── SERVER-TO-SERVER TRANSFER ───────────────────────

func transferRemoteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var req struct {
		SourceConnID int      `json:"source_conn_id"`
		SourceFiles  []string `json:"source_files"`
		DestConnID   int      `json:"dest_conn_id"`
		DestPath     string   `json:"dest_path"`
		OnConflict   string   `json:"on_conflict"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", 400)
		return
	}
	if req.SourceConnID == 0 || req.DestConnID == 0 || len(req.SourceFiles) == 0 {
		jsonError(w, "source_conn_id, dest_conn_id, and source_files are required", 400)
		return
	}
	srcAccess, code, msg := authorizeConnection(r, req.SourceConnID, PermFilesRead)
	if srcAccess == nil {
		jsonError(w, msg, code)
		return
	}
	dstAccess, code, msg := authorizeConnection(r, req.DestConnID, PermFilesWrite)
	if dstAccess == nil {
		jsonError(w, msg, code)
		return
	}
	if len(req.SourceFiles) > 10000 {
		jsonError(w, "Too many files selected", 400)
		return
	}

	srcConn, err := loadConnection(req.SourceConnID)
	if err != nil {
		jsonError(w, "Source connection not found", 404)
		return
	}
	dstConn, err := loadConnection(req.DestConnID)
	if err != nil {
		jsonError(w, "Destination connection not found", 404)
		return
	}
	actorID, actorName := srcAccess.actor()
	auditLogRef(r, actorID, actorName, "file.transfer", srcConn.Name+" → "+dstConn.Name,
		map[string]interface{}{"files": len(req.SourceFiles), "dest_path": req.DestPath, "to_conn_id": dstConn.ID}, auditRef{ConnID: srcConn.ID})

	if isFTP(srcConn) || isFTP(dstConn) || isWeb(srcConn) || isWeb(dstConn) || isDesktopProtocol(srcConn.Protocol) || isDesktopProtocol(dstConn.Protocol) {
		jsonError(w, "Server-to-server transfer needs SSH/SFTP connections on both sides", 400)
		return
	}

	// Setup streaming response IMMEDIATELY so frontend gets feedback
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)

	sendInitial := func(msg interface{}) {
		json.NewEncoder(w).Encode(msg)
		if canFlush {
			flusher.Flush()
		}
	}

	ctx := r.Context()
	clientGone := ctx.Done()

	// Connect to source SFTP
	sendInitial(map[string]interface{}{"type": "status", "message": "Connecting to source server..."})
	srcSSHClient, err := getSSHClient(srcConn)
	if err != nil {
		sendInitial(map[string]interface{}{"type": "error", "error": "Source SSH failed: " + err.Error()})
		return
	}
	defer srcSSHClient.Close()
	sendInitial(map[string]interface{}{"type": "status", "message": "Source SSH connected. Opening SFTP..."})
	srcSFTP, err := sftp.NewClient(srcSSHClient)
	if err != nil {
		sendInitial(map[string]interface{}{"type": "error", "error": "Source SFTP failed: " + err.Error()})
		return
	}
	defer srcSFTP.Close()
	sendInitial(map[string]interface{}{"type": "status", "message": "Source ready. Connecting to destination..."})

	// Connect to destination SFTP
	dstSSHClient, err := getSSHClient(dstConn)
	if err != nil {
		sendInitial(map[string]interface{}{"type": "error", "error": "Destination SSH failed: " + err.Error()})
		return
	}
	defer dstSSHClient.Close()
	sendInitial(map[string]interface{}{"type": "status", "message": "Destination SSH connected. Opening SFTP..."})
	dstSFTP, err := sftp.NewClient(dstSSHClient)
	if err != nil {
		sendInitial(map[string]interface{}{"type": "error", "error": "Destination SFTP failed: " + err.Error()})
		return
	}
	defer dstSFTP.Close()
	sendInitial(map[string]interface{}{"type": "status", "message": "Both servers connected. Starting transfer..."})

	if req.DestPath != "" && req.DestPath != "." {
		dstSFTP.MkdirAll(req.DestPath)
	}

	var sendMu sync.Mutex
	sendMsg := func(msg interface{}) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		select {
		case <-clientGone:
			return fmt.Errorf("client disconnected")
		default:
		}
		if err := json.NewEncoder(w).Encode(msg); err != nil {
			return err
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	}

	type transferResult struct {
		File   string `json:"file"`
		Status string `json:"status"`
		Size   int64  `json:"size"`
		Error  string `json:"error,omitempty"`
	}

	// Ensure directory exists on destination
	ensureDir := func(dirPath string) error {
		if dirPath == "" || dirPath == "/" || dirPath == "." {
			return nil
		}
		info, err := dstSFTP.Stat(dirPath)
		if err == nil {
			if info.IsDir() {
				return nil
			}
			dstSFTP.Remove(dirPath)
		}
		if err := dstSFTP.MkdirAll(dirPath); err != nil {
			return fmt.Errorf("mkdir %s: %v", dirPath, err)
		}
		info2, err := dstSFTP.Stat(dirPath)
		if err != nil || !info2.IsDir() {
			return fmt.Errorf("cannot create directory %s", dirPath)
		}
		return nil
	}

	// Transfer a single file + stream results
	transferFile := func(srcPath, dstPath string) transferResult {
		result := transferResult{File: srcPath, Status: "ok"}

		srcFileObj, err := srcSFTP.Open(srcPath)
		if err != nil {
			result.Status = "failed"
			result.Error = "Cannot open source: " + err.Error()
			return result
		}

		srcInfo, err := srcFileObj.Stat()
		if err != nil {
			srcFileObj.Close()
			result.Status = "failed"
			result.Error = "Cannot stat source: " + err.Error()
			return result
		}
		result.Size = srcInfo.Size()

		// Send file_start before beginning transfer
		sendMsg(map[string]interface{}{
			"type": "file_start",
			"file": srcPath,
			"size": srcInfo.Size(),
		})

		dstDir := path.Dir(dstPath) // remote paths always use "/", even when WRM runs on Windows
		if err := ensureDir(dstDir); err != nil {
			srcFileObj.Close()
			result.Status = "failed"
			result.Error = "Cannot create directory: " + err.Error()
			return result
		}

		// Check conflict
		if dstInfo, err := dstSFTP.Stat(dstPath); err == nil && !dstInfo.IsDir() {
			switch req.OnConflict {
			case "skip":
				srcFileObj.Close()
				result.Status = "skipped"
				result.Size = 0
				return result
			case "newer":
				if !srcInfo.ModTime().After(dstInfo.ModTime()) {
					srcFileObj.Close()
					result.Status = "skipped"
					result.Size = 0
					return result
				}
			case "abort":
				srcFileObj.Close()
				result.Status = "conflict"
				result.Error = "File exists: " + dstPath
				return result
			}
		}

		dstFileObj, err := dstSFTP.Create(dstPath)
		if err != nil {
			srcFileObj.Close()
			result.Status = "failed"
			result.Error = "Cannot create destination: " + err.Error()
			return result
		}

		buf := make([]byte, 1024*1024)
		var written int64
		sum := sha256.New()
		logResult := func() {
			st := "ok"
			if result.Status != "ok" {
				st = "failed"
			}
			logFileTransfer(r, srcAccess, transferRec{Direction: "s2s", SrcConn: &srcConn, SrcPath: srcPath, DstConn: &dstConn, DstPath: dstPath,
				Size: written, SHA256: hex.EncodeToString(sum.Sum(nil)), Status: st, Error: result.Error})
		}
		defer logResult()
		lastProgress := time.Now()
		for {
			select {
			case <-clientGone:
				srcFileObj.Close()
				dstFileObj.Close()
				result.Status = "failed"
				result.Error = "Client disconnected"
				return result
			default:
			}
			n, readErr := srcFileObj.Read(buf)
			if n > 0 {
				nw, writeErr := dstFileObj.Write(buf[:n])
				if writeErr != nil {
					srcFileObj.Close()
					dstFileObj.Close()
					result.Status = "failed"
					result.Error = "Write error: " + writeErr.Error()
					return result
				}
				written += int64(nw)
				sum.Write(buf[:nw])
				// Send progress every 500ms
				if time.Since(lastProgress) >= 500*time.Millisecond {
					sendMsg(map[string]interface{}{
						"type":    "file_progress",
						"file":    srcPath,
						"written": written,
						"total":   srcInfo.Size(),
					})
					lastProgress = time.Now()
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					srcFileObj.Close()
					dstFileObj.Close()
					result.Status = "failed"
					result.Error = "Read error: " + readErr.Error()
					return result
				}
				break
			}
		}
		srcFileObj.Close()
		if err := dstFileObj.Close(); err != nil {
			result.Status = "failed"
			result.Error = "Write error: " + err.Error()
			return result
		}

		if !srcInfo.ModTime().IsZero() {
			dstSFTP.Chtimes(dstPath, srcInfo.ModTime(), srcInfo.ModTime())
		}

		log.Printf("Transfer OK: %s -> %s (%d bytes)", srcPath, dstPath, written)
		return result
	}

	// Collect all files to transfer (flatten dirs)
	type fileJob struct {
		srcPath string
		dstPath string
	}
	var jobs []fileJob

	for _, srcFile := range req.SourceFiles {
		fileName := path.Base(strings.TrimSuffix(srcFile, "/"))
		dstFullPath := path.Join(req.DestPath, fileName)

		srcInfo, err := srcSFTP.Stat(srcFile)
		if err != nil {
			r := transferResult{File: srcFile, Status: "failed", Error: "Source not found: " + err.Error()}
			sendMsg(map[string]interface{}{"type": "file", "result": r})
			continue
		}

		if srcInfo.IsDir() {
			ensureDir(dstFullPath)
			var collectFiles func(string)
			collectFiles = func(dir string) {
				entries, err := srcSFTP.ReadDir(dir)
				if err != nil {
					r := transferResult{File: dir, Status: "failed", Error: "Cannot list: " + err.Error()}
					sendMsg(map[string]interface{}{"type": "file", "result": r})
					return
				}
				for _, entry := range entries {
					sPath := dir + "/" + entry.Name()
					dPath := path.Join(dstFullPath, strings.TrimPrefix(sPath, srcFile))
					if entry.IsDir() {
						collectFiles(sPath)
					} else {
						jobs = append(jobs, fileJob{srcPath: sPath, dstPath: dPath})
					}
				}
			}
			collectFiles(srcFile)
		} else {
			jobs = append(jobs, fileJob{srcPath: srcFile, dstPath: dstFullPath})
		}
	}

	// Recount total size from actual jobs
	var totalSize int64
	for _, j := range jobs {
		if info, err := srcSFTP.Stat(j.srcPath); err == nil {
			totalSize += info.Size()
		}
	}

	// Send progress with correct file count
	if err := sendMsg(map[string]interface{}{
		"type":      "progress",
		"total":     len(jobs),
		"totalSize": totalSize,
	}); err != nil {
		log.Printf("Transfer cancelled: client disconnected")
		return
	}

	// Transfer files with worker pool (4 concurrent workers)
	const numWorkers = 4
	var mu sync.Mutex
	okCount, failCount, skipCount := 0, 0, 0
	var errorList []string
	var clientDisconnected atomic.Bool
	jobCh := make(chan fileJob, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				if clientDisconnected.Load() {
					return
				}
				select {
				case <-clientGone:
					clientDisconnected.Store(true)
					return
				default:
				}
				r := transferFile(j.srcPath, j.dstPath)
				mu.Lock()
				sendMsg(map[string]interface{}{"type": "file", "result": r})
				switch r.Status {
				case "ok":
					okCount++
				case "skipped":
					skipCount++
				default:
					failCount++
					if r.Error != "" {
						errorList = append(errorList, j.srcPath+": "+r.Error)
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Send done
	if !clientDisconnected.Load() {
		sendMsg(map[string]interface{}{
			"type":      "done",
			"ok":        okCount,
			"failed":    failCount,
			"skipped":   skipCount,
			"errorList": errorList,
		})
	}
}

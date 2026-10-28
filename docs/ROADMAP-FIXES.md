# WRM PRO â€” roadmap popravaka (2026-10)

**Cilj:** zatvoriti nalaze iz "fresh eyes" pregleda v13.14.1, po prioritetu, s testom za svaki popravak.
**Izvor istine:** `wrm-pro-enterprise` (v14.x, Apache-2.0). Javno izdanje (`web-browser-RDM-public`, v13.x) dobiva iste popravke cherry-pickom; svaki red ima oba commita.
**Tracking:** [enterprise #7](https://github.com/vedranius/wrm-pro-enterprise/issues/7) · [public #139](https://github.com/vedranius/web-browser-RDM-public/issues/139)

**Pravilo:** jedan popravak = jedan commit = jedan test. Commit poruka: `fix(<area>): <ID> <short description>` i `Refs #<tracking issue>`.

Legenda statusa: `todo` Â· `wip` Â· `done` Â· `n/a` (samo enterprise / samo public)

## Sigurnost

| ID | Severity | Problem | Popravak | Test | Status | Enterprise commit | Public commit |
|----|----------|---------|----------|------|--------|-------------------|---------------|
| SEC-01 | High | 2FA nema lockout; IP se resetira prije drugog faktora (`users.go`) | uspjeh biljeĹľi tek nakon 2FA; pogreĹˇan TOTP/recovery diĹľe `failed_logins` i `locked_until` | test: N pogreĹˇnih TOTP zakljuÄŤava raÄŤun; uspjeh resetira | todo | â€” | â€” |
| SEC-02 | Medium | Ponovna provjera lozinke bez rate-limita (password, export, reveal, keys, 2FA disable) | zajedniÄŤki `confirmPassword` s per-account + per-IP limitom i auditom | test: 6. pokuĹˇaj â†’ 429; ispravna lozinka prolazi | todo | â€” | â€” |
| SEC-03 | High | SSRF: Git source, NetBox, AI provider `base_url`, OIDC discovery | zajedniÄŤki safe dialer (blokira loopback/RFC1918/link-local/ULA/metadata) + admin allowlist | test: `127.0.0.1`, `169.254.169.254`, `10.0.0.1` odbijeni; javni host prolazi | todo | â€” | â€” |
| SEC-04 | High | Loopback tuneli dostupni svim korisnicima; SOCKS bez autha | provjera vlasnika na svim loopback dialovima; SOCKS user/pass | test: drugi korisnik ne moĹľe na tuÄ‘i tunel | todo | â€” | â€” |
| SEC-05 | Medium | Globalni host-key TOFU | known_hosts po vlasniku; auto-trust traĹľi vlasniĹˇtvo | test: korisnik B ne moĹľe auto-trustati host korisnika A | todo | â€” | â€” |
| SEC-06 | Medium | Audit lanac bez HMAC-a; CLI stvara dvije grane | HMAC s kljuÄŤem izvan DB-a; insert ÄŤita prethodni hash | test: izmijenjen zapis â†’ verify fail; CLI insert ne lomi lanac | todo | â€” | â€” |
| SEC-07 | Medium | Gost identitet je klijentski kolaÄŤiÄ‡; `allow_link_shares=off` proĹˇiruje na sve prijavljene | potpisani guest cookie + IP binding; zabranjeni link = 403, ne `users` | test: obrisan cookie ne vraÄ‡a pristup; policy off â†’ 403 | todo | â€” | â€” |
| SEC-08 | Low | CSP `unsafe-inline`; HSTS samo na TLS-u; cookie bez `__Host-` | nonce CSP, HSTS preko `isHTTPS`, `__Host-` + rotacija | test: headeri; CSP bez `unsafe-inline` | todo | â€” | â€” |
| SEC-09 | Medium | MCP: DCR consent phishing, token upgrade bez step-upa, revoke bez autha | potvrda podrijetla + step-up za poveÄ‡anje prava; revoke auth | test: upgrade bez step-upa â†’ 403; tuÄ‘i revoke â†’ 401 | todo | â€” | â€” |
| SEC-10 | Medium | Addon ZIP bez ukupnog limita; `%2e%2e`; secrets rade kad je addon iskljuÄŤen | ukupni cap + fail umjesto truncate; dekodirati putanju; enabled gate iznad dispatcha | test: 500Ă—8 MB odbijeno; `%2e%2e` odbijen; disabled â†’ 409 | todo | â€” | â€” |

## AI / MCP

| ID | Severity | Problem | Popravak | Test | Status | Enterprise | Public |
|----|----------|---------|----------|------|--------|------------|--------|
| AI-01 | High | `sudo -u root rm -rf /`, `rm -rf ../..`, here-string zaobilaze listu | unwrap `sudo/doas` s vrijednostima; normalizirati relativne putanje; parsirati `<<<` | test: sve tri forme â†’ `rm_root`/`deny` | todo | â€” | â€” |
| AI-02 | High | Auto budĹľet se resetira po sessionu; approve-later bez re-evaluacije; race na `s.Auto` | perzistirati `Used/Until` u grant; re-evaluirati s trenutnim limitima; kopirati pod lockom | test: reopen ne daje novih 50 akcija; later poĹˇtuje deny | todo | â€” | â€” |
| AI-03 | Medium | `curl` je read-only (egress bez odobrenja); read-deny ne pokriva `.env`/`*.pem` | mreĹľni egress = promjena ili `ai_allow_network`; proĹˇiriti deny listu | test: `.env` i `curl` van â†’ approval | todo | â€” | â€” |
| AI-04 | Low | Sistemski prompt prima `PRETTY_NAME` bez untrusted oznake; nova datoteka zaobilazi read-deny | sanitizirati/escape; primijeniti read-deny i na nove putanje | test: newline/metakaraktery u os-info | todo | â€” | â€” |

## Podaci i deploy

| ID | Severity | Problem | Popravak | Test | Status | Enterprise | Public |
|----|----------|---------|----------|------|--------|------------|--------|
| DATA-01 | High | `foreign_keys` samo na jednoj pooled konekciji; nema `SetMaxOpenConns` | `_pragma=foreign_keys(1)` u DSN; ograniÄŤen pool | test: FK prekrĹˇaj odbijen na viĹˇe konekcija | todo | â€” | â€” |
| DATA-02 | High | Bundle manifest bez validacije (`../`, apsolutne putanje) | validacija pri importu + re-validacija prije upotrebe | test: bundle s `../` odbijen | todo | â€” | â€” |
| DATA-03 | Medium | `ensureTable` netransakcijski; enterprise nema 13.14.1 fix | transakcija + `createColumns` + restore instanci | test: restart ne gubi `addon_instances` | todo | â€” | â€” |
| DATA-04 | Low | Brisanje konekcije ostavlja monitoring/log/git siroÄŤad | `deleteConnectionRows()` na svim mjestima | test: nema redova nakon delete | todo | â€” | â€” |
| GIT-01 | High | Host build = shell za svakoga s `git_medium`, nasljeÄ‘uje `WRM_*` env | admin-only + scrub env | test: ne-admin â†’ 403; env bez `WRM_*` | todo | â€” | â€” |
| GIT-02 | Medium | 2 MB scan cap; `/tmp` deploy log; tar bez validacije | stream/po instalaciji; maknuti `/tmp`; `tar -tf` provjera | test: veliki scan; symlink log; tar s `..` | todo | â€” | â€” |

## Frontend / UX

| ID | Severity | Problem | Popravak | Test | Status | Enterprise | Public |
|----|----------|---------|----------|------|--------|------------|--------|
| UX-01 | High | 19 `fetch()` bez `res.ok` â†’ laĹľni uspjeh | `apiJSON()` wrapper; svi delete/revoke/rename | test: 403 prikaĹľe greĹˇku, ne uspjeh | todo | â€” | â€” |
| UX-02 | High | 1,27 MB HTML bez gzipa/ETag; vendor JS blokira paint | gzip+ETag middleware; `defer`; SW network-first | test: `Content-Encoding`, 304, defer | todo | â€” | â€” |
| UX-03 | High | a11y: 231 label bez `for`, 59 ikona bez imena, 0 `aria-live`, bez focus trapa | `for/id`, `aria-label`, `role=status`, modal helper | test: axe/lighthouse prag; ruÄŤna provjera | todo | â€” | â€” |
| UX-04 | Medium | onboarding/empty state, mijeĹˇani jezik, "AI connections"/"Medium"/"Session" | copy + first-run kartica + EN/HR na auth | test: vizualna provjera | todo | â€” | â€” |
| UX-05 | Medium | `esc` u inline handlerima; clipboard u localStorage; addon bridge | `jsq()`, TTL+filtar, frame prefix | test: ime s `'` ne izvrĹˇava JS | todo | â€” | â€” |
| QA-01 | Medium | Windows test suite ne prolazi; CI samo Ubuntu | GOOS skipovi + windows-latest job | CI zelen na oba OS-a | todo | â€” | â€” |
| ARCH-01 | Medium | Nema deny-by-default autorizacije (150+ ruta) | middleware s allowlistom javnih ruta + test enumeracije | test: nova ruta bez auth pada test | todo | â€” | â€” |

## Enterprise-only

| ID | Problem | Popravak | Status |
|----|---------|----------|--------|
| ENT-01 | WebAuthn `UserVerification: preferred` (passkey moĹľe biti single-factor) | `required` za 2FA/passkey prijavu; dokumentirati | todo |
| ENT-02 | Backup CLI kopira `.key` uz backup | zadrĹľati, ali jasno upozoriti + opcija `--without-key` | todo |
| ENT-03 | Nema scheduliranog backupa | dnevni backup + retencija (admin policy) | todo |

## Redoslijed izdavanja

1. **v14.0.1 / v13.14.2** â€” SEC-01, AI-01, DATA-01, DATA-02, UX-01, UX-02, DATA-03 (merge 13.14.1 fix).
2. **v14.1.0 / v13.15.0** â€” SEC-02â€¦SEC-10, AI-02â€¦AI-04, GIT-01/02, UX-03â€¦05.
3. **v14.2.0** â€” ARCH-01, QA-01, ENT-01â€¦03.

> Svaki release: tag na enterprise `main`, cherry-pick istih commitova na public `main`, tag `v13.x`, provjera `git diff --stat` izmeÄ‘u tagova (oÄŤekivane razlike: licence, enterprise moduli).

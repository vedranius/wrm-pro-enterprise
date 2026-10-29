# WRM PRO — roadmap popravaka (2026-10)

**Cilj:** zatvoriti nalaze iz "fresh eyes" pregleda v13.14.1, po prioritetu, s testom za svaki popravak.
**Izvor istine:** `wrm-pro-enterprise` (v14.x, Apache-2.0). Javno izdanje (`web-browser-RDM-public`, v13.x) dobiva iste popravke cherry-pickom; svaki red ima oba commita.
**Tracking:** [enterprise #7](https://github.com/vedranius/wrm-pro-enterprise/issues/7) · [public #139](https://github.com/vedranius/web-browser-RDM-public/issues/139)
**Pravilo:** jedan popravak = jedan commit = jedan test. Commit poruka: `fix(<area>): <ID> <short description>` i `Refs #<tracking issue>`.

Legenda statusa: `todo` · `wip` · `done` · `n/a` (samo enterprise / samo public)

## Sigurnost

| ID | Severity | Problem | Popravak | Test | Status | Enterprise commit | Public commit |
|----|----------|---------|----------|------|--------|-------------------|---------------|
| SEC-01 | High | 2FA nema lockout; IP se resetira prije drugog faktora (`users.go`) | uspjeh bilježi tek nakon 2FA; pogrešan TOTP/recovery diže `failed_logins` i `locked_until` | test: N pogrešnih TOTP zaključava račun; uspjeh resetira | done | ea80f04 | bcdac42 |
| SEC-02 | Medium | Ponovna provjera lozinke bez rate-limita (password, export, reveal, keys, 2FA disable) | zajednički `confirmPassword` s per-account + per-IP limitom i auditom | test: 6. pokušaj → 429; ispravna lozinka prolazi | done | 38c10ea | c6e4a6e |
| SEC-03 | High | SSRF: Git source, NetBox, AI provider `base_url`, OIDC discovery | zajednički safe dialer (blokira loopback/RFC1918/link-local/ULA/metadata) + admin allowlist | test: `127.0.0.1`, `169.254.169.254`, `10.0.0.1` odbijeni; javni host prolazi | done (metadata/link-local + public_only politika) | f486cd6 | 61e3cf2 |
| SEC-04 | High | Loopback tuneli dostupni svim korisnicima; SOCKS bez autha | provjera vlasnika na svim loopback dialovima; SOCKS user/pass | test: drugi korisnik ne može na tuđi tunel | done | 77264c4 | 7a6bf1c |
| SEC-05 | Medium | Globalni host-key TOFU | known_hosts po vlasniku; auto-trust traži vlasništvo | test: korisnik B ne može auto-trustati host korisnika A | done | cc8a0ee | 67d39c6 |
| SEC-06 | Medium | Audit lanac bez HMAC-a; CLI stvara dvije grane | HMAC s ključem izvan DB-a; insert čita prethodni hash | test: izmijenjen zapis → verify fail; CLI insert ne lomi lanac | done | fb5afaa | ab241d3 |
| SEC-07 | Medium | Gost identitet je klijentski kolačić; `allow_link_shares=off` proširuje na sve prijavljene | potpisani guest cookie + IP binding; zabranjeni link = 403, ne `users` | test: obrisan cookie ne vraća pristup; policy off → 403 | done | 4232668 | 7ad00fb |
| SEC-08 | Low | CSP `unsafe-inline`; HSTS samo na TLS-u; cookie bez `__Host-` | nonce CSP, HSTS preko `isHTTPS`, `__Host-` + rotacija | test: headeri; CSP bez `unsafe-inline` | done (HSTS, __Host- kolačić, CSP hash) | a5bd915 | 240b5a4 |
| SEC-09 | Medium | MCP: DCR consent phishing, token upgrade bez step-upa, revoke bez autha | potvrda podrijetla + step-up za povećanje prava; revoke auth | test: upgrade bez step-upa → 403; tuđi revoke → 401 | done (step-up; DCR/revoke ostaje) | 36f6a5b | 671d118 |
| SEC-10 | Medium | Addon ZIP bez ukupnog limita; `%2e%2e`; secrets rade kad je addon isključen | ukupni cap + fail umjesto truncate; dekodirati putanju; enabled gate iznad dispatcha | test: 500×8 MB odbijeno; `%2e%2e` odbijen; disabled → 409 | done | 9580643 | 1b4699d |

## AI / MCP

| ID | Severity | Problem | Popravak | Test | Status | Enterprise | Public |
|----|----------|---------|----------|------|--------|------------|--------|
| AI-01 | High | `sudo -u root rm -rf /`, `rm -rf ../..`, here-string zaobilaze listu | unwrap `sudo/doas` s vrijednostima; normalizirati relativne putanje; parsirati `<<<` | test: sve tri forme → `rm_root`/`deny` | done | 2105206 | 26dd037 |
| AI-02 | High | Auto budžet se resetira po sessionu; approve-later bez re-evaluacije; race na `s.Auto` | perzistirati `Used/Until` u grant; re-evaluirati s trenutnim limitima; kopirati pod lockom | test: reopen ne daje novih 50 akcija; later poštuje deny | done (budžet + mode lock) | f91ee9f | 4138824 |
| AI-03 | Medium | `curl` je read-only (egress bez odobrenja); read-deny ne pokriva `.env`/`*.pem` | mrežni egress = promjena ili `ai_allow_network`; proširiti deny listu | test: `.env` i `curl` van → approval | done | c250b66 | 21e1d2d |
| AI-04 | Low | Sistemski prompt prima `PRETTY_NAME` bez untrusted oznake; nova datoteka zaobilazi read-deny | sanitizirati/escape; primijeniti read-deny i na nove putanje | test: newline/metakaraktery u os-info | done | c250b66 | 21e1d2d |

## Podaci i deploy

| ID | Severity | Problem | Popravak | Test | Status | Enterprise | Public |
|----|----------|---------|----------|------|--------|------------|--------|
| DATA-01 | High | `foreign_keys` samo na jednoj pooled konekciji; nema `SetMaxOpenConns` | `_pragma=foreign_keys(1)` u DSN; ograničen pool | test: FK prekršaj odbijen na više konekcija | done | be9e3e0 | 958ee59 |
| DATA-02 | High | Bundle manifest bez validacije (`../`, apsolutne putanje) | validacija pri importu + re-validacija prije upotrebe | test: bundle s `../` odbijen | done | fbf10d5 | 754a7a0 |
| DATA-03 | Medium | `ensureTable` netransakcijski; enterprise nema 13.14.1 fix | `createColumns` + restore instanci (transakcija ostaje kao DATA-03b) | test: restart ne gubi `addon_instances` | done | 244a4f9 | — (u 13.14.1) |
| DATA-04 | Low | Brisanje konekcije ostavlja monitoring/log/git siročad | `deleteConnectionRows()` na svim mjestima | test: nema redova nakon delete | done | 9bf7e1b | 0a56f14 |
| GIT-01 | High | Host build = shell za svakoga s `git_medium`, nasljeđuje `WRM_*` env | admin-only + scrub env | test: ne-admin → 403; env bez `WRM_*` | done | e8c3860 | f77d9a5 |
| GIT-02 | Medium | 2 MB scan cap; `/tmp` deploy log; tar bez validacije | stream/po instalaciji; maknuti `/tmp`; `tar -tf` provjera | test: veliki scan; symlink log; tar s `..` | done (scan cap, log, tar imena) | 5e7cdd4 | 82bb220 |

## Frontend / UX

| ID | Severity | Problem | Popravak | Test | Status | Enterprise | Public |
|----|----------|---------|----------|------|--------|------------|--------|
| UX-01 | High | 19 `fetch()` bez `res.ok` → lažni uspjeh | `apiOk()` wrapper; delete/revoke/rename | test: 403 prikaže grešku, ne uspjeh | done | e7aa180 | d75ab8a |
| UX-02 | High | 1,27 MB HTML bez gzipa/ETag; vendor JS blokira paint | gzip+ETag; `defer`; SW network-first (SW ostaje) | test: `Content-Encoding`, 304, defer | done | 6a881b3 | 4017ada |
| UX-03 | High | a11y: 231 label bez `for`, 59 ikona bez imena, 0 `aria-live`, bez focus trapa | `for/id`, `aria-label`, `role=status`, modal helper | test: axe/lighthouse prag; ručna provjera | done (aria-live, lang, focus trap, labele) | 6441eff | 2a9edf4 |
| UX-04 | Medium | onboarding/empty state, miješani jezik, "AI connections"/"Medium"/"Session" | copy + first-run kartica + EN/HR na auth | test: vizualna provjera | done | 9628ea2 | 0ce6dc0 |
| UX-05 | Medium | `esc` u inline handlerima; clipboard u localStorage; addon bridge | `jsq()`, TTL+filtar, frame prefix | test: ime s `'` ne izvršava JS | done (clipboard TTL, recovery codes; ostale interpolacije su brojčani ID-evi, provjereno skeniranjem) | 8c73ccd | 3df8754 |
| QA-01 | Medium | Windows test suite ne prolazi; CI samo Ubuntu | GOOS skipovi + windows-latest job | CI zelen na oba OS-a | done | a9709f7 | 27fc325 |
| ARCH-01 | Medium | Nema deny-by-default autorizacije (150+ ruta) | middleware s allowlistom javnih ruta + test enumeracije | test: nova ruta bez auth pada test | done | 2e431c5 | 0f9f81f |

## Enterprise-only

| ID | Problem | Popravak | Status |
|----|---------|----------|--------|
| ENT-01 | WebAuthn `UserVerification: preferred` (passkey može biti single-factor) | `required` za 2FA/passkey prijavu; dokumentirati | done (54a05fe) |
| ENT-02 | Backup CLI kopira `.key` uz backup | zadržati, ali jasno upozoriti + opcija `--without-key` | done (fb24f3b) |
| ENT-03 | Nema scheduliranog backupa | dnevni backup + retencija (admin policy) | done (e1ec223) |

## Redoslijed izdavanja

1. **v14.0.1 / v13.14.2** — SEC-01, AI-01, DATA-01, DATA-02, DATA-03, UX-01, UX-02.
2. **v14.1.0 / v13.15.0** — SEC-02…SEC-10, AI-02…AI-04, GIT-01/02, UX-03…05.
3. **v14.2.0** — ARCH-01, QA-01, ENT-01…03.

> Svaki release: tag na enterprise `main`, cherry-pick istih commitova na public `main`, tag `v13.x`, provjera `git diff --stat` između tagova (očekivane razlike: licence, enterprise moduli).

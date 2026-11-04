# WRM PRO - roadmap: Storages (S3)

**Cilj:** S3-kompatibilni objektni storage kao zaseban dio proizvoda (**Storages**), odvojen od
Connections: dodavanje, pregled po prefiksima, preview, reprodukcija videa, download, **uređivanje
malih tekstualnih objekata** (JSON, konfiguracije, titlovi) i brisanje samo kad je izričito
dopušteno. Sve unutar WRM-a: dozvole, audit, limiti, AI/MCP.

**Što ovo nije:** transfer alat. Kroz WRM se ne šalju veliki fileovi; tipičan posao je ispraviti
JSON, popraviti liniju titla, obrisati zastarjeli objekt i provjeriti je li MP4 onaj pravi.
Zato nema multiparta, nema „upload mape”, nema transkodiranja.

**Izvor istine:** `wrm-pro-enterprise` (RFC `docs/rfcs/0008-storages-s3.md`). Javno izdanje
(`web-browser-RDM-public`) dobiva iste promjene; svaki red ima oba commita.

**Pravilo:** jedan popravak = jedan commit = jedan test. Commit: `feat(storages): STG-xx ...`
i `Refs #19` (public: `Refs #155`).

**Tracking:** [enterprise #19](https://github.com/vedranius/wrm-pro-enterprise/issues/19) - [public #155](https://github.com/vedranius/web-browser-RDM-public/issues/155)

**Feature flag:** `storages_enabled` (default `off`), vidljivo na security levelu
`business`/`enterprise`.

**Procjena cjeline:** MVP (STG-01 … STG-08) **≈ 26 radnih dana**; ostatak (STG-09 … STG-12)
**+10 dana**. Nakon STG-08 može se izdati upotrebljiv v14.1.0.

## Faze i verzije

| Faza | Stavke | Verzija (enterprise / public) |
|---|---|---|
| A - temelj | STG-01, STG-02 | v14.1.0-alpha1 / v13.15.0-alpha1 (neobjavljeno) |
| B - backend i pregled | STG-03, STG-04 | v14.1.0-alpha2 / v13.15.0-alpha2 |
| C - editor i video | STG-05, STG-06 | v14.1.0-beta / v13.15.0-beta |
| D - sigurnost i izdanje | STG-07, STG-08 | **v14.1.0 / v13.15.0** |
| E - integracije | STG-09, STG-10, STG-11 | v14.2.0 / v13.16.0 |
| F - otvrdnjavanje | STG-12 | v14.2.1 |

## Stavke

| ID | Faza | Što | Popravak | Test | Status | Dani |
|----|------|-----|----------|------|--------|------|
| STG-01 | A | Nema modela za storage | tablice `storages`, `storage_shares`; `ensureTable`; `encryptValue` za secret; audit `storage.created/updated/deleted`; setting `storages_enabled` | `TestStorageCRUDAndSecretMasking`: secret se nikad ne vraća, audit zapisan, tablice nastaju na startu | done (3cd5a2f) | 2 |
| STG-02 | A | Nema S3 klijenta | `storage_s3.go`: SigV4, `ListObjectsV2`, `GetObject` (Range), `HeadObject`, `PutObject` (limit tijela), `CopyObject`, `DeleteObject(s)`, retry/timeout, mapiranje grešaka | `TestS3ClientAgainstStub`: list/range/get/put/copy/delete + provjera potpisa na stubu | done (3cd5a2f) | 3 |
| STG-03 | B | Nema ruta ni granica | `/api/storages…`, `requireStorage(cap)`, konfinacija prefiksa, validacija ključa, limiti tijela, rate limit, `If-Match`/409 | `TestStorageRoutesRespectCapabilities`: read-only odbija PUT/DELETE; ključ s `..` odbijen; prefiks se ne može napustiti | done (3cd5a2f) | 4 |
| STG-04 | B | Nema UI | sekcija **Storages** (nav, lista, empty state), modal s presetima (Hetzner/DO/AWS/MinIO), **Test connection**, prozor: breadcrumb, lista, preview, download, „dodaj datoteku”, novi folder, rename, akcija brisanja | `node tools/check-ui.js`; ručni tok: dodaj → pregledaj → download → obriši | done (e2a7068) | 4 |
| STG-05 | C | Nema editora | editor: JSON (format + validacija s linijom/kolonom), YAML/XML/INI, skripte, **SRT/VTT/ASS/SSA** s provjerom vremena i „pomakni titlove”; čuvanje encodiga i CRLF/LF; `If-Match` uz „reload / overwrite”; limit `max_edit_kb` (2 MB) | `TestStorageEditorSave`: BOM/CRLF ostaju, 409 na promijenjen etag, 413 iznad limita; `TestSubtitleChecks`: monotonost i renumber | done (e2a7068) | 5 |
| STG-06 | C | Nema videa ni titlova | player preko `GET /object?inline=1` (Range, seek); `media-src 'self'`; poruka za nepodržani kodek; bočni `x.srt`/`x.vtt`/`x.ass` kao track (SRT/ASS → VTT **u browseru**, izvorni file se ne dira) | `TestStorageRangeStreaming`: 206, `Content-Range`, seek preko granice dijela; ručna provjera reprodukcije + titla | done (e2a7068) | 3 |
| STG-07 | D | Brisanje bez zaštite | `allow_delete` (default off) + `X-WRM-Confirm: delete` + step-up kad je 2FA uključen + audit (ključ, veličina); multi-select uz dodatnu potvrdu; UI skriva akciju | `TestStorageDeleteGate`: bez dozvole 403, bez headera 400, s dozvolom 200 i audit; step-up 403 bez svježe potvrde | done (29ef811) | 2 |
| STG-08 | D | Nema testova ni dokumentacije | S3 stub za sve operacije; TLS pin test; „secret ne izlazi” (API, audit, export, log); README sekcija za operatere | `go test -race -run TestStorage` zeleno; stub pokriva Range, 409 i greške providera | done (3cd5a2f) | 3 |
| STG-09 | E | AI/MCP ne vidi storage | scopeovi `storage_read/write/delete`, alati `storage_list/read/write/delete`, AI read-deny lista (`.env`, `*.pem`), write samo uz scope i `allow_write`, delete uz scope + `allow_delete` + `confirm` | `TestStorageMCPTools`: read prolazi, write bez scopea odbijen, delete bez potvrde/dozvole odbijen, `.env` odbijen | done (c7f3f44) | 3 |
| STG-10 | E | Nema dijeljenja | share link na prefiks: token hash, `can_write`/`can_delete`, expiry, read-only default; revoke pri brisanju storagea | `TestStorageShareToken`: token otvara samo svoj prefiks, istekli 410, revoke 401 | todo | 3 |
| STG-11 | E | Nema izvoza ni statusa | config export/import s `storages` (secreti redigirani, pri importu se traže ponovno); status check = `HeadBucket`; notifikacija kad storage padne | `TestStorageExportRedactsSecretsAndImport`, `TestStorageStatusRefresh` | done (29ef811, e5f0369; notifikacija ostaje) | 2 |
| STG-12 | F | Otvrdnjavanje | update `docs/THREAT-MODEL.md`, PSIRT pregled, provjera SSRF/TLS/preview/deletion puta, pentest scenariji | sigurnosni pregled potpisan; checklist iz RFC-a odštikliran | djelomično (threat model; PSIRT pregled ostaje) | 2 |

## Definicija "done" (po stavci)

1. Kod + test koji pada prije i prolazi poslije.
2. `gofmt`, `go vet`, `node tools/check-ui.js` (kad dira UI).
3. Audit za svaku promjenu i svako odbijanje; secret nikad u logu, API-ju ni exportu.
4. Dozvola provjerena **na serveru**, ne u UI-u.
5. Zapis u `docs/ROADMAP-STORAGES.md` (status + SHA) i komentar na tracking issue.
6. Patch/minor verzija i release notes kad se faza zatvori.

## Sigurnosne provjere prije v14.1.0

- [x] Secret se ne pojavljuje u nijednom odgovoru, logu, audit detalju ni exportu.
- [x] `prefix` se ne može napustiti nijednim ključem (`..`, Unicode normalizacija, `%2e%2e`).
- [x] Endpoint prolazi `guardEgressURL`; LAN/MiniIO traži `egress_allow_hosts`.
- [x] TLS pin radi; bez pina self-signed cert pada.
- [x] HTML/SVG/JS/XML se nikad ne renderiraju inline u WRM originu (samo `text/plain` ili attachment).
- [x] Brisanje: dozvola + potvrda + audit + step-up; UI ne nudi akciju kad nije dopuštena.
- [x] Editor: 409 na promijenjen etag, 413 iznad limita, BOM/CRLF se čuvaju.
- [x] Video: Range/206, bez transkodiranja; nepodržan kodek jasno prijavljen.
- [x] Rate limit i max veličina objekta rade.
- [ ] Share token: hash, prefiks, expiry, revoke.
- [x] AI: read uz `storage_read`; write uz scope i `allow_write`; delete uz scope + `allow_delete` + `confirm`; `.env`/`*.pem` odbijeni.

## Rizici

| Rizik | Utjecaj | Ublažavanje |
|---|---|---|
| Razlike među providerima (Hetzner, DO, MinIO, AWS) | neke operacije rade na jednom, ne na drugom | zajednički podskup (list/get/head/put/copy/delete); matrica kompatibilnosti u RFC-u |
| Kodek koji browser ne podržava (HEVC) | „video se ne vidi” | jasna poruka + download; bez transkodiranja u v1 |
| Veliki MP4 | RAM/prekidi | Range streaming, nikad se ne učitava cijeli objekt |
| Opasni formati (SVG/HTML) | XSS u WRM originu | nikad inline; `text/plain` ili attachment; reuse postojećih preview pravila |
| Titlovi u različitim formatima | krivo vrijeme/encoding | provjere u editoru, čuvanje BOM/CRLF, konverzija samo za prikaz |
| Dugotrajni secrets | curenje | trezor, write-only, rotacija + podsjetnik, audit |

## Otvorena pitanja (odluka prije STG-01)

1. Vidi li administrator sve storageove korisnika (kao konekcije) ili samo svoje? *Predlog: kao konekcije.*
2. Treba li „undelete” (verzioniranje) u v2 ili je dovoljan hint providera? *Predlog: v2.*
3. Treba li `ffprobe` metadata linija (trajanje, rezolucija) kad je ffmpeg dostupan? *Predlog: opcionalno, van v1.*

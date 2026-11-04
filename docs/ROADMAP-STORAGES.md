# WRM PRO - roadmap: Storages (S3)

**Cilj:** S3-kompatibilni objektni storage kao zaseban dio proizvoda (**Storages**), odvojen od
Connections: dodavanje, pregled, preview, download, upload i uređivanje, uz brisanje samo kad
je izričito dopušteno. Sve unutar WRM-a: dozvole, audit, limiti, AI/MCP.

**Izvor istine:** `wrm-pro-enterprise` (RFC `docs/rfcs/0008-storages-s3.md`). Javno izdanje
(`web-browser-RDM-public`) dobiva iste promjene; svaki red ima oba commita.

**Pravilo:** jedan popravak = jedan commit = jedan test. Commit: `feat(storages): STG-xx ...`
i `Refs #19` (public: `Refs #155`).

**Tracking:** [enterprise #19](https://github.com/vedranius/wrm-pro-enterprise/issues/19) - [public #155](https://github.com/vedranius/web-browser-RDM-public/issues/155)

**Feature flag:** `storages_enabled` (default `off`), vidljivo na security levelu
`business`/`enterprise`.

**Procjena cjeline:** MVP (STG-01 … STG-06) **≈ 21 radni dan**; ostatak (STG-07 … STG-10)
**+10 dana**. Redoslijed je takav da se nakon STG-06 može izdati upotrebljiv v14.1.0.

## Faze i verzije

| Faza | Stavke | Verzija (enterprise / public) |
|---|---|---|
| A - temelj | STG-01, STG-02 | v14.1.0-alpha1 / v13.15.0-alpha1 (neobjavljeno) |
| B - backend | STG-03, STG-04 | v14.1.0-alpha2 / v13.15.0-alpha2 |
| C - sigurnost | STG-05, STG-06 | v14.1.0-beta / v13.15.0-beta |
| D - izdanje | STG-07, STG-08, STG-09 | **v14.1.0 / v13.15.0** |
| E - otvrdnjavanje | STG-10 | v14.1.1 |

## Stavke

| ID | Faza | Što | Popravak | Test | Status | Dani |
|----|------|-----|----------|------|--------|------|
| STG-01 | A | Nema modela za storage | tablice `storages`, `storage_shares`; `ensureTable`; `encryptValue` za secret; audit `storage.created/updated/deleted`; setting `storages_enabled` | `TestStorageCRUDAndSecretMasking`: secret se nikad ne vraća, audit zapisan, dvije tablice nastaju na startu | todo | 3 |
| STG-02 | A | Nema S3 klijenta | `storage_s3.go`: SigV4, `ListObjectsV2`, `GetObject` (Range), `PutObject` (multipart >16 MiB), `HeadObject`, `CopyObject`, `DeleteObject(s)`, retry/timeout, mapiranje grešaka (AccessDenied, SignatureDoesNotMatch, NoSuchBucket) | `TestS3ClientAgainstStub`: list/range/get/put/multipart/copy/delete + potpis provjeren na stubu | todo | 4 |
| STG-03 | A/B | Nema ruta | `/api/storages…` (vidi RFC), `requireStorage(cap)`, konfinacija prefiksa, validacija ključa, limiti tijela, rate limit, `If-Match` | `TestStorageRoutesRespectCapabilities`: read-only storage odbija PUT/DELETE (403); ključ s `..` odbijen; prefiks se ne može napustiti | todo | 4 |
| STG-04 | B | Nema UI | sekcija **Storages** (nav, lista, empty state), modal s presetima (Hetzner/DO/AWS/MinIO), **Test connection**, prozor s breadcrumbom, preview, download, drag&drop upload, novi folder, rename, editor sa `If-Match` | `node tools/check-ui.js`; ručna provjera toka (dodavanje → pregled → edit → upload) | todo | 5 |
| STG-05 | C | Brisanje bez zaštite | `allow_delete` (default off) + `X-WRM-Confirm: delete` + step-up kad je 2FA uključen + audit s ključem i veličinom; UI skriva akciju | `TestStorageDeleteGate`: bez dozvole 403, bez headera 400, s dozvolom 200 i audit; step-up 403 bez svježe potvrde | todo | 2 |
| STG-06 | C | Nema testova | lokalni S3 stub (`httptest`) za sve operacije; TLS pin test (self-signed + `ca_fingerprint`); test da secret ne izlazi u API/export/log | `go test -race -run TestStorage` zeleno; stub pokriva multipart i Range | todo | 3 |
| STG-07 | D | AI/MCP ne vidi storage | scopeovi `storage_read/write/delete`, alati `storage_list/read/write/delete`, read-deny lista (`.env`, `*.pem`), write kroz approval, delete odbijen bez scopea | `TestStorageAIToolsAndScopes`: read prolazi, write traži approval, delete bez scopea odbijen | todo | 3 |
| STG-08 | D | Nema dijeljenja | share link na prefiks: token hash, `can_write`/`can_delete`, expiry, read-only default; revoke pri brisanju storagea | `TestStorageShareToken`: token otvara samo svoj prefiks, istekli 410, revoke 401 | todo | 3 |
| STG-09 | D | Nema izvoza ni statusa | config export/import s `storages` (secreti redigirani, pri importu se traže ponovno); status check = `HeadBucket`; notifikacija kad storage padne | `TestStorageExportRedactsSecrets`, `TestStorageStatusCheck` | todo | 2 |
| STG-10 | E | Otvrdnjavanje | update `docs/THREAT-MODEL.md`, PSIRT pregled, provjera SSRF/TLS/deletion puta, pentest scenariji, dokumentacija za operatere | sigurnosni pregled potpisan; checklist u RFC-u odštikliran | todo | 2 |

## Definicija "done" (po stavci)

1. Kod + test koji pada prije i prolazi poslije.
2. `gofmt`, `go vet`, `node tools/check-ui.js` (kad dira UI).
3. Audit za svaku promjenu i svako odbijanje; secret nikad u logu, API-ju ni exportu.
4. Dozvola provjerena **na serveru**, ne u UI-u.
5. Zapis u `docs/ROADMAP-STORAGES.md` (status + SHA) i komentar na tracking issue.
6. Patch/minor verzija i release notes kad se faza zatvori.

## Sigurnosne provjere prije v14.1.0

- [ ] Secret se ne pojavljuje u nijednom odgovoru, logu, audit detalju ni exportu.
- [ ] `prefix` se ne može napustiti nijednim ključem (`..`, Unicode normalizacija, `%2e%2e`).
- [ ] Endpoint prolazi `guardEgressURL`; LAN/MiniIO traži `egress_allow_hosts`.
- [ ] TLS pin radi; bez pina self-signed cert pada.
- [ ] Brisanje: dozvola + potvrda + audit + step-up; UI ne nudi akciju kad nije dopuštena.
- [ ] Rate limit i max veličina objekta rade; multipart se prekida na grešci.
- [ ] Share token: hash, prefiks, expiry, revoke.
- [ ] AI: write traži approval, delete odbijen bez scopea; read-deny lista vrijedi.

## Rizici

| Rizik | Utjecaj | Ublažavanje |
|---|---|---|
| Razlike među providerima (Hetzner, DO, MinIO, AWS) | neke operacije rade na jednom, ne na drugom | držati se zajedničkog podskupa (list/get/put/head/copy/delete/multipart); matrica kompatibilnosti u RFC-u |
| Veliki objekti | RAM i prekidi | streaming, multipart 8 MiB, abort, max_object_mb |
| Dugotrajni secrets | curenje | trezor, write-only, rotacija + podsjetnik, audit |
| Zamjena postojećih ruta | regresija file managera | zaseban `/api/storages` namespace; ništa postojeće se ne mijenja |
| Očekivanje "obriši mapu" | slučajno brisanje | u v1 brisanje jednog objekta uz potvrdu; brisanje prefiksa samo kroz multi-select s dodatnom potvrdom |

## Otvorena pitanja (odluka prije STG-01)

1. Vidi li administrator sve storageove korisnika (kao konekcije) ili samo svoje? *Predlog: kao konekcije.*
2. Treba li "undelete" (verzioniranje) u v2 ili je dovoljan hint providera? *Predlog: v2.*
3. Presigned upload za velike datoteke? *Predlog: ne u v1, izmjeriti pa odlučiti.*

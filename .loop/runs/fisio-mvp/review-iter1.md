## Review Feedback — Iteration 1

Verdicts: Clean Architecture APPROVED (0C/0W/6S) · OO Design APPROVED (0C/2W/7S) · API Design CHANGES_REQUESTED (1C/5W/11S).

### MUST FIX (CRITICAL / bugs confirmed by E2E in Chromium)

- **API-001 (CRITICAL)** `gauth.Client(ctx)` passes a short-lived ctx to `cfg.TokenSource(ctx, tok)` and `oauth2.NewClient(ctx, ...)`; callers (`main.go` 60s timeout ctx, `handlers_auth.go` request ctx) cancel it right after the factory returns. oauth2 uses that ctx for every refresh (`internal/token.go:259`), so after ~1h every Google call fails with `context canceled`. Fix: TokenSource/NewClient must use a process-lifetime ctx (e.g. `context.Background()` or a ctx stored at `gauth.New`); keep bounded ctx only for the bootstrap calls. `ServicesFactory` should take a lifetime ctx; `ConnectServices(r *http.Request)` → `ConnectServices(ctx)`. Add a test: build Client with an already-cancelled ctx, assert a refresh via fake token endpoint still succeeds.
- **E2E-001 (HIGH)** Full-page GET `/agenda/novo` (and likely `/agenda/{id}/editar`) renders a form with `hx-target="#agenda-main"`, but `#agenda-main` does not exist on that page → htmx fires `htmx:targetError` and never submits (proved with Playwright: no POST hits the server). Fix: on full-page render either omit hx-* attributes (plain POST → 303 already works) or wrap the page in an element with `id="agenda-main"`. Add a web test asserting the full-page form has no dangling hx-target.
- **E2E-002 (HIGH)** Toast text is mojibake ("EvoluÃ§Ã£o registrada"): HX-Trigger header carries raw UTF-8 and the browser decodes headers as Latin-1. Fix: build the header with `json.Marshal` and escape non-ASCII to `\uXXXX` (set `encoder.SetEscapeHTML`, then post-process runes > 0x7F), so the header is pure ASCII. Replace hand-rolled `jsonString`. Test: header must be ASCII-only for a message with "ção".
- **E2E-003 (MEDIUM)** Week view: appointment chips overflow their day column (desktop 1280px, Sunday column text spills outside the card). Fix in app.css: `min-width:0` on grid children, `overflow:hidden; text-overflow:ellipsis; white-space:nowrap` on chip text, or wrap. Verify by screenshot.
- **E2E-004 (LOW)** Date headings show "Quinta, 22 De Outubro De 2026" and "Semana De 19/10 A 25/10" — CSS `text-transform: capitalize` capitalises prepositions. Remove the transform and capitalise only the weekday/first letter in the template func.

### WARNINGS (fix; cross-reviewer or straightforward)

- **OO-001 / API-002 / CA-004** `ErrMirror` success-with-error tri-state: return `WriteResult{Appointment, MirrorWarn error}` (error nil on partial success) from Schedule/Reschedule/Cancel/MarkDone/Link; collapse the five `errors.Is(err, app.ErrMirror)` checks; `fail()` must never see ErrMirror; keep the underlying Sheets error unwrappable.
- **OO-002 / CA-003** Web layer calls `time.Now()` directly (handlers_agenda.go 45/69/81, handlers_patients.go 39/85/135/153, http.go 106). Add `Clock domain.Clock` to `web.Options`, default `domain.ClockFunc(time.Now)`, use `s.clock.Now()`; pin the clock in web tests.
- **API-003** `Agenda.ListBetween(from,to)` semantics differ: memory = Start in [from,to); gcalendar = Google overlap semantics. Document on the port ("Start in [from,to)") and filter in gcalendar after mapping. Add a shared contract test run against both adapters (memory + fake EventsAPI).
- **API-004** Rename `PatientService.Record` → `GetRecord`.
- **API-005** `ValidationError.Add` doc claims nil-safety it lacks: fix doc or make nil-safe.
- **API-006** Saving from the week view swaps in the day partial under URL /agenda/semana. Use a hidden `vista` field (or HX-Current-URL) to render `agenda-week` when originating from the week view; add a test.

### SUGGESTIONS (do the cheap ones; skip if they widen scope)
- CA-001/OO-004 move "Fisio: <name>" rule to `domain.Patient.SessionTitle()`; use in app + seed.
- CA-002 single `domain.DefaultSessionDuration` constant used by app + web.
- CA-005/OO-006 move `RandomIDs` out of gsheets (e.g. `internal/adapters/ids` or domain).
- CA-006 delete dead `AgendaService.Location()`.
- OO-003/API-007 drop unused `AppointmentMirror.Delete` (and its chain) and `PatientRepository.Search` (filter in service via MatchesQuery).
- OO-005 remove duplicated "Informe data e hora" pre-checks; rely on Validate.
- OO-007 `app.Services` + `app.NewServices(...)` bundle; web references `*app.Services`.
- OO-008 dedupe `patientFromInput` vs `applyPatientInput`; `fieldError` template helper.
- API-008 `EvolutionInput` struct; API-009 `Cancel` returns Appointment; API-012 `gauth.New(Config)`.
- API-014 validate `voltar` as a local path (single leading "/", no "//").
- API-015 styled catch-all 404; `requireConnection` → 303 for non-GET.
- API-011 `ParseStatus` returns ok bool; row mappers log bad cells.
- Reviewer note: `gcalendar.ListBetween` should skip (and log) a single unparsable event instead of failing the whole view.
- OO-009, API-010, API-013, API-016, API-017: optional; skip unless trivial.

### Verified correct (do not touch)
Import graph is a clean DAG (domain ← app/adapters ← web ← cmd), ports in domain, narrow SDK seams (EventsAPI/SheetsAPI), HTMX status contract (303 plain POST, 200+HX-Redirect, 422+HX-Retarget, 404 via fail), demo seed, mobile layout.

## Verificador adversarial (independente) — VERDICT: FAIL
Scratch com provas: /tmp/claude-0/-home-user-deposito/8b82bf38-7203-5224-9575-c55810b9e4c9/scratchpad/verifier/ (shots/, pw/, overlay/).

- **V-CRIT-1** = API-001 (ctx cancelado no TokenSource; provado com overlay test: `context canceled` no refresh).
- **V-HIGH-1** Salvar pelo modal (Novo atendimento/Remarcar) NÃO atualiza a agenda: `toast()` manda `HX-Trigger {"toast","closeModal"}`; htmx processa HX-Trigger ANTES do retarget; `closeModal` esvazia `#modal-body`, o form é destacado do DOM e o alvo `#agenda-main` resolve como null → `Cannot read properties of null (reading 'classList')`. Toast aparece, modal fecha, lista fica velha (reload mostra o item). Fix: mandar `closeModal` em `HX-Trigger-After-Swap` (ou fechar no `htmx:afterRequest` do form); `toast` pode ficar no HX-Trigger.
- **V-HIGH-2** "Agendar" na ficha do paciente não envia nada (form `hx-target="#agenda-main"`, elemento inexistente em patient_record.html → htmx:targetError, POST nunca sai). Mesmo problema em `/agenda/{id}/editar` página inteira e `/agenda/novo` (E2E-001). Fix robusto: form do modal com `hx-target="#modal-body"` (existe em todas as páginas) e o servidor decide via HX-Retarget/HX-Redirect; página inteira sem hx-*.
- **V-HIGH-3** Células numéricas editadas por humano na planilha voltam como float64 → `fmt.Sprint` dá `1.1987654321e+10` (api.go:36,44 usa UNFORMATTED_VALUE). Fix: `FORMATTED_VALUE`, ou formatar float64 com `strconv.FormatFloat(f,'f',-1,64)`; teste com célula float. Também: ao criar a planilha setar `Properties.Locale="pt_BR"` e `TimeZone` para que datas digitadas saiam dd/MM/yyyy.
- **V-MED-1** = E2E-002 mojibake no toast.
- **V-MED-2** Limpar Observações/Local no Remarcar nunca limpa no Calendar: Patch com `omitempty` sem `ForceSendFields` (mapping.go:223-234). Fix: `ForceSendFields: []string{"Description","Location"}`; ao converter all-day→horário usar `NullFields` para `Start.Date`/`End.Date`.
- **V-MED-3** "Sincronizar" apaga linhas `cancelado` do espelho (ReplaceAll com eventos vivos; Cancel apaga o evento). README promete manter. Fix: merge preservando linhas cancelado (ou documentar; preferir merge).
- **V-MED-4** `-addr` padrão `:8080` escuta em todas as interfaces; POSTs sem checagem de origem (CSRF: `curl -H "Origin: https://evil.example" -d nome=CSRF /pacientes → 303`). Fix: padrão `127.0.0.1:8080`; middleware rejeitando POST cujo `Origin`/`Sec-Fetch-Site` não seja same-origin (403).
- **V-MED-5** Semana em 390px: viewport cresce para 647px (grid `minmax(140px,1fr)` + `.mini` sem clip). Fix: `.day-card{overflow:hidden}`, `.mini{min-width:0;overflow:hidden;text-overflow:ellipsis}`; em telas estreitas empilhar dias (1 coluna) em vez de 7.
- **V-MED-6** Agrupamento da semana errado em fusos com DST (`Sub(first).Hours()/24` e `AddDate`), provado com America/Santiago. Fix: índice por comparação (Y,M,D) e dias com `time.Date(y,m,d+i,0,0,0,0,loc)`.
- **V-LOW** `jsonString` não escapa chars de controle (TAB no nome quebra o JSON do header) → usar `json.Marshal`; open redirect em `voltar` (aceitar só path começando com "/" e não "//"); sem `http.MaxBytesReader` (nome de 9MB aceito) → limitar corpo (ex. 64KB) e tamanho de campos; `http.Server` sem Read/Write/IdleTimeout; `Cache-Control` de estáticos sem versionamento (usar query `?v=<hash>` ou max-age curto); `timeMin` do Google filtra por END, memória por START (= API-003: filtrar por Start em [from,to) após o mapeamento); evento sem summary renderiza título vazio (mostrar "(sem título)"); página de conexão diz "só planilhas que ele criar" mas pede escopo `spreadsheets` completo (corrigir texto; `drive.file` redundante → remover); após token revogado o gate continua passando e tudo vira 500 (detectar erro de auth e voltar à tela "Conectar"); README não documenta `-token`/`FISIO_TOKEN`; `.env.example` diz "copie para .env" mas nada carrega .env (corrigir texto: exportar variáveis ou usar flags); "Cancelar" oferecido em sessão CONCLUÍDO (esconder); Remarcar de concluído mantém status concluído (voltar para agendado); badge `st-agendado` contraste 4.03:1 (<4.5); botões de ação 34px (<44px alvo de toque).
- **Testes faltando**: caminho ErrMirror na camada web (usar memory.Mirror.Fail), escaping do HX-Trigger, célula float no Sheets, Patch com ForceSendFields, refresh do gauth com ctx cancelado, Week em fuso com DST, e um teste de navegador do fluxo modal (teria pego V-HIGH-1/2).

### Verificado correto pelo verificador (não mexer sem motivo)
Rotas HX/full-page, 303/422/404/405, flash cookie, connect gate e OAuth state/CSRF do callback, escaping HTML, parsing datetime-local em loc, concorrência (-race, 300 req paralelas), modal a11y (focus trap, Esc, backdrop), estados vazios, mobile 390px fora da semana, dark mode, gcalendar (cancelled, paging, reminders, props), gsheets (índices, bootstrap idempotente), gauth (0600, refresh persistido).

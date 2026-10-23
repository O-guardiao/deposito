# Evidências — fisio-mvp (iteração 1, implementador)

Ambiente: go1.24.7 linux/amd64, GOTOOLCHAIN=local. Data: 2026-10-23.

## Build / vet / fmt
```
$ gofmt -l .            → (vazio)
$ go build ./...        → ok
$ go vet ./...          → VET_OK
```

## Testes (com -race)
```
$ go test -race -count=1 ./...
ok  internal/adapters/gauth      1.021s
ok  internal/adapters/gcalendar  1.020s
ok  internal/adapters/gsheets    1.021s
ok  internal/app                 1.013s
ok  internal/web                 1.104s
(cmd/fisio, memory, domain: sem arquivos de teste — cobertos indiretamente)
```
Cobertura funcional dos testes:
- app: validação, Schedule (título/endereço/espelho), ErrMirror preserva evento,
  MarkDone/Cancel (remoção + linha cancelada), Reschedule mantém duração, Link de
  evento solto, Week agrupa por dia no fuso, Record separa passado/futuro,
  Search/List ordenados, Reconcile copia só vinculados.
- gcalendar: round-trip evento↔appointment (props privadas, lembretes 1440/60, TZ),
  evento sem vínculo, evento dia-inteiro, 404→ErrNotFound, status cancelled filtrado.
- gsheets: round-trip de linhas, linhas curtas e dd/mm/aaaa, Bootstrap cria planilha/
  abas/cabeçalhos e é idempotente, upsert atualiza no lugar, Get/Search/NotFound,
  evoluções por paciente, espelho upsert/delete/replaceAll.
- gauth: carrega credentials.json (installed), URL de login com state/redirect,
  ErrNotConnected sem token, token em disco conecta.
- web (httptest, demo): dia/semana/raiz, partial vs página, criar paciente (303 +
  HX-Redirect + flash cookie + 422), busca filtra, ficha/404/editar, fluxo de
  agendamento via HTMX (form, retarget, 422 no modal, redirect sem JS, `voltar`),
  concluir/remarcar/vincular/cancelar/404/sincronizar, evoluções (200/422/404),
  gate “Conectar com Google”, /auth/login + callback (state inválido 400, sucesso
  constrói serviços).

## Smoke do servidor em modo demo (`fisio -demo -addr :8089`)
```
/agenda                          200 10889B
/agenda/semana                   200 7911B
/pacientes                       200 9041B
/pacientes/P0001                 200 6592B
/pacientes/novo                  200 4080B
/agenda/novo?data=2026-10-23     200 4660B
/agenda/evt0001/editar           200 4335B
/pacientes/nope                  404 2855B
/healthz                         200 2B
/pacientes?q=maria               200 4653B
HX GET /pacientes?q=jo           200 1157B (partial)
HX POST /pacientes               200 + Hx-Redirect: /pacientes/P0006
HX POST /agenda (válido)         200 + Hx-Retarget #agenda-main, Hx-Trigger toast
HX POST /agenda (inválido)       422 + Hx-Retarget #modal-body, "Escolha o paciente"
HX POST /sincronizar             204 + toast "Planilha sincronizada: 13 atendimento(s)"
HX POST /pacientes/P0001/evolucoes 200 + toast, partial #evolutions
log do servidor: sem "error"/"panic"
```

## Não comprovado nesta iteração
- Integração real com Google (sem credenciais no ambiente): adaptadores compilam
  contra `google.golang.org/api v0.267.0` e são testados com fakes.
- Experiência no navegador (modal/toast/JS, responsivo): só HTML via curl/httptest.

## Iteração 2 — verificação (2026-10-23)

Comandos: `gofmt -l .`, `go vet ./...`, `go mod tidy -diff`, `go test -race -count=1 -cover ./...` → ver bloco abaixo (gofmt vazio, vet ok, tidy sem diff, todos os pacotes ok).

Navegador (Chromium/Playwright) contra `fisio -demo -addr 127.0.0.1:8089`:
- Suíte própria `scratchpad/e2e/run.mjs` (mobile 390px + desktop 1280px): **fails: 0** em 23 passos por viewport, incluindo: salvar pelo modal atualiza a agenda sem reload e fecha o modal; toast "Agendado para 22/10 às 18:45" sem mojibake; zero `pageerror`; Remarcar pelo modal atualiza a lista; "Agendar" na ficha cria e mostra em Próximos atendimentos; salvar a partir da semana mantém o grid da semana e a URL /agenda/semana; semana sem overflow horizontal (scrollWidth 390 = innerWidth 390).
- Script do verificador (`verifier/pw/e2e2.mjs`, porta ajustada): B2 "appointments before 12 after save (no reload) 13 … after reload 13"; B3 remarcar "06:15" sem reload e após reload; B4 concluir 2→3; G nome com TAB/aspas → toast e vínculo funcionam, sem XSS; J mobile: agenda/week/patients/record/form/empty todos `scrollWidth 390 innerWidth 390 ok`; K contraste light mínimo 4.84 (badge agendado 6.69), dark mínimo 6.76.
- Screenshots em `scratchpad/e2e/shots/` (dia, semana, ficha, modal, dark).
gofmt -l: []
go vet: ok
go mod tidy -diff: empty
ok  	github.com/o-guardiao/deposito/internal/adapters/gauth	1.040s	coverage: 78.1% of statements
ok  	github.com/o-guardiao/deposito/internal/adapters/gcalendar	1.029s	coverage: 78.1% of statements
ok  	github.com/o-guardiao/deposito/internal/adapters/gsheets	1.028s	coverage: 66.3% of statements
ok  	github.com/o-guardiao/deposito/internal/adapters/ids	1.015s	coverage: 100.0% of statements
ok  	github.com/o-guardiao/deposito/internal/adapters/memory	1.013s	coverage: 33.3% of statements
ok  	github.com/o-guardiao/deposito/internal/app	1.020s	coverage: 80.8% of statements
ok  	github.com/o-guardiao/deposito/internal/web	1.158s	coverage: 80.1% of statements

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

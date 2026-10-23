# Decisões — fisio-mvp (iteração 1, implementador)

1. **Sem banco próprio.** Google Calendar = fonte da agenda; Google Sheets = pacientes,
   evoluções e espelho de agendamentos. Leitura sempre ao vivo (sem cache): volume de
   uma profissional, simplicidade > latência.
2. **Ordem de gravação: agenda primeiro, planilha depois.** Falha na planilha vira
   `app.ErrMirror` (aviso na UI, atendimento preservado). Botão “Sincronizar” reconstrói
   a aba Agendamentos a partir do Calendar (-90d..+180d).
3. **Vínculo evento↔paciente** por `extendedProperties.private` (`fisio_app`,
   `fisio_patient_id`, `fisio_status`). Evento sem a propriedade = “marcado fora do
   app”, com ação Vincular. Cancelar = deletar evento no Google + linha `cancelado`
   na planilha (histórico preservado).
4. **Lembretes** fixos por política (`ReminderPolicy`, padrão popup 1440 e 60 min),
   `UseDefault=false` com `ForceSendFields` para o JSON enviar o `false`.
5. **Portas no domínio** (`domain/ports.go`); adaptadores Google atrás de interfaces
   mínimas (`gcalendar.EventsAPI`, `gsheets.SheetsAPI`) para testar mapeamento e
   fluxo sem rede. Fakes nos testes; `memory` para demo.
6. **Dependências pinadas para Go 1.24**: `google.golang.org/api v0.267.0` e
   `golang.org/x/oauth2 v0.33.0` (versões mais novas exigem go 1.25/1.26 e disparariam
   download de toolchain na máquina dela). `go mod tidy` reescreve `go 1.24.0` +
   `toolchain go1.24.7`; mantido.
7. **HTMX**: partial vs página inteira decidido pelo header `HX-Request`; todas as
   rotas funcionam sem JS (POST → 303). 422 re-renderiza formulário com
   `HX-Retarget: #modal-body`; sucesso em agenda devolve o partial do dia com
   `HX-Retarget: #agenda-main` + `HX-Trigger` (toast + fechar modal). Redirecionamentos
   usam cookie flash para o toast sobreviver. `htmx-config` via meta habilita swap em 422.
8. **IDs**: demo `P0001`/`E0001` (legíveis); produção `P-AAAAMMDD-XXXXXXXX` (random,
   sortável por dia, seguro se dois processos gravarem).
9. **OAuth loopback** com `state` em cookie HttpOnly; token em `data/token.json` (0600),
   refresh persistido. `FISIO_PUBLIC_URL` precisa bater com o redirect cadastrado.
10. **Fuso**: `time/tzdata` embutido; `America/Sao_Paulo` padrão; dias calculados no fuso
    (teste cobre 23:30 local ≠ dia seguinte em UTC).
11. **Observação de processo**: apareceu um commit `8be72fc WIP…` durante a
    implementação que NÃO foi feito pelo implementador (diretriz era não commitar).

## Iteração 2 (correções da revisão + verificador)

- **API-001/V-CRIT-1** `gauth.Authenticator` guarda um contexto base (processo) e `Client()` não recebe mais ctx: o `TokenSource` e o `oauth2.NewClient` usam esse contexto, então o refresh sobrevive ao fim do request/setup. `ServicesFactory(ctx)` documenta que ctx limita só o bootstrap; o callback OAuth usa `context.WithoutCancel(r.Context())` + timeout para o bootstrap não ser cortado se o navegador fechar (evitaria planilha criada sem ID salvo).
- **V-HIGH-1** `closeModal` vai em `HX-Trigger-After-Swap`; `HX-Trigger` só leva o toast. Assim o form continua no DOM até o retarget resolver.
- **V-HIGH-2/E2E-001** Formulário do modal aponta para `#modal-body` (existe em toda página); o servidor decide o destino via `HX-Retarget #agenda-main` (dia ou semana, conforme campo oculto `vista`) ou `HX-Redirect` (campo `voltar`, ficha). Renderizado como página inteira, o form não tem atributos hx-* (POST normal → 303).
- **V-HIGH-3** `ReadRows` usa `FORMATTED_VALUE` + `cellString` (float sem notação científica); planilha criada com `Locale=pt_BR` e `TimeZone` do app; `parseDate` aceita `dd/mm/aaaa hh:mm`.
- **OO-001/API-002/CA-004** `WriteResult{Appointment, MirrorWarning}`: erro de espelho deixa de ser erro; `fail()` nunca vê esse caso.
- **OO-002/CA-003** `web.Options.Clock`; testes web fixam "hoje" em 22/10/2026 14:00.
- **API-003** Contrato de `ListBetween` (Start em [from,to)) documentado na porta e garantido por `domaintest.RunAgendaContract`, rodado pelos adaptadores memory e gcalendar (fake com semântica de overlap do Google).
- **V-MED-6** `domain.DayStart/NextDay/SameDay`: dias por aritmética de calendário; dia sem meia-noite (DST) começa no primeiro instante existente. Teste com America/Santiago.
- **V-MED-3** Porta `AppointmentMirror.Reconcile(live)` substitui `ReplaceAll`+`Delete`: linhas `cancelado` sobrevivem ao Sincronizar.
- **V-MED-4** Padrão `127.0.0.1:8080`; middleware same-origin (Origin/Sec-Fetch-Site) devolve 403 em POST de outra origem; `MaxBytesReader` 64 KiB; timeouts no `http.Server`.
- **V-MED-2** `ForceSendFields` Description/Location; `forPatch` anula `start.date/end.date` em eventos com horário.
- Demais: `GetRecord`, `EvolutionInput`, `app.Services/NewServices(Deps)`, `app.Window`, `Patient.SessionTitle()`, `domain.DefaultSessionDuration`, `ids.Random{Clock}` fora do gsheets, `Search` fora da porta (filtro no serviço), `fieldError` no template, `pageMeta` por página, `localPath` para `voltar`, 404 estilizado em `GET /{path...}` (405 preservado), gate devolve 303 em não-GET, `?v=<hash>` nos estáticos com `immutable`, token revogado → `Forget` + tela Conectar, `(sem título)`, Cancelar oculto em concluído, remarcar concluído → agendado, contraste do badge, alvos de toque 44px, semana empilhada no celular.
- **Não feito (de propósito)**: API-013 (renomear rotas para /atendimentos — churn sem ganho para uso individual); API-016 (chaves de validação = nomes de campo — mantido o mapeamento explícito nos templates); mapeadores de linha continuam lenientes (API-011) — registrar célula inválida exigiria logger nos mapeadores puros; aviso de conflito de horário (fora do contrato).

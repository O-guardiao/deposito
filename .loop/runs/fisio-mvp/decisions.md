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

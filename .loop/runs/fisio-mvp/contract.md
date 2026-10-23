# Contrato — fisio-mvp

## Objetivo
App web (Go + HTMX) para fisioterapeuta domiciliar: pacientes com ID único e ficha,
evoluções (notas de sessão), agenda com lembretes. Google Sheets = base de pacientes/
evoluções/espelho de agendamentos; Google Calendar = fonte da agenda (mão dupla:
o app grava nos dois; lê agendamentos ao vivo do Calendar, então eventos criados no
celular aparecem no app).

## Escopo (MVP)
- Pacientes: criar, editar, listar, buscar por nome, ficha completa.
- Evoluções: adicionar e listar por paciente.
- Agenda: visão dia e semana, criar/remarcar/cancelar/concluir, vincular evento
  solto a paciente, lembretes (1 dia antes + 1h antes).
- Sincronizar Calendar → aba Agendamentos da planilha.
- Modo demo (`-demo`) com adaptadores em memória (MOCK, claramente rotulado).
- OAuth2 com conta Google dela; bootstrap da planilha (cria abas se faltarem).

## Fora de escopo
- Banco SQL (não necessário). Multiusuário. Notificações por WhatsApp.

## Restrições
- Go 1.24 stdlib (net/http, html/template, embed) + google.golang.org/api.
- HTMX 2.0.4 vendorado (unpkg/jsdelivr bloqueados; obtido do GitHub).
- Sem credenciais Google no ambiente: integração real NÃO comissionada aqui.

## Critérios de aceite
1. `go build ./... && go vet ./... && go test ./...` verdes.
2. `fisio -demo` sobe; fluxo E2E em navegador: criar paciente → agendar → ver na
   agenda → adicionar evolução → ficha mostra tudo (screenshots como evidência).
3. Adaptadores Google compilam contra a API real e têm testes de mapeamento.
4. README com passo a passo de credenciais Google e execução.
5. Revisão arquitetural (arxitect) sem CRITICAL; verificador adversarial independente.

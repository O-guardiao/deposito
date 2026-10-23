# Frontend Engineering Audit: Redesign com Tailwind v4 + SVG (frontend didático)

**Perspective**: Frontend Engineer
**Date**: 2026-10-23
**Plan**: pedido do usuário ("design aplicado, Tailwind v4 e SVG, moderno e didático porque a cliente é leiga") aplicado ao frontend atual em `internal/web/` (Go `html/template` + HTMX 2.0.4, sem React)

> Calibração: este skill é escrito para React/Zustand/TypeScript. O projeto é
> server-rendered (Go templates + HTMX), então cada lente foi traduzida para o
> equivalente: componente = template/partial, estado = DOM + swaps HTMX,
> render path = ciclo requisição → partial → swap, type safety = contrato
> entre structs de view e templates, bundle = assets embutidos no binário.

## Plan Summary
Trocar a folha de estilo artesanal (194 linhas, variáveis CSS) por Tailwind v4 compilado em build (CSS estático commitado e embutido), manter HTMX e os contratos de DOM existentes, e redesenhar os templates com foco em uma usuária leiga: navegação sempre rotulada, textos explicativos, legenda de status, página "Como funciona" com diagrama SVG, estados vazios ilustrados e formulários com dicas.

## Files Reviewed
| File | Frontend Role | Risk |
|------|--------------|------|
| `internal/web/templates/layout.html` | shell: topbar, nav, modal `<dialog>`, toast, htmx-config | Médio (ids `#modal`, `#modal-body`, `#toast`, `#main` são contrato do `app.js` e dos handlers) |
| `internal/web/templates/partials/agenda.html` | `agenda-day`, `appointment-item`, `agenda-week`, `appointment-form` | Alto (alvos `#agenda-main`/`#modal-body`, campos `paciente/inicio/duracao/observacoes/voltar/vista`, classe `status-*` usada em teste) |
| `internal/web/templates/partials/patients.html` | `patient-list`, `evolutions`, `error` | Médio (`#patient-list`, `#evolutions`, campos `data/texto`) |
| `internal/web/templates/pages/*.html` | páginas completas (ficha, formulário, lista, conectar, erro) | Baixo |
| `internal/web/templates/partials/icons.html` | ícones SVG inline | Baixo |
| `internal/web/static/app.css` | tokens, componentes, dark mode automático, reduced motion | Alto (será gerado pelo Tailwind) |
| `internal/web/static/app.js` | toast, modal, listeners HTMX (`toast`, `closeModal`, `afterSwap`, erros) | Baixo |
| `internal/web/render.go` | FuncMap pt-BR, `asset` com hash, embed | Médio (novas funções `shortTitle`, `plural`) |
| `internal/web/http.go` | `pages` (título/seção), `render` partial-vs-página | Baixo (nova página `help`) |

## Verdict: APPROVE WITH CHANGES

## Critical Issues (Must Fix)
| # | Issue | File(s) | Why It Breaks | Code Fix |
|---|-------|---------|---------------|----------|
| 1 | Classes dinâmicas montadas no template (`status-{{.Status}}`, `st-{{.Status}}`) não são detectáveis pelo scanner do Tailwind | `partials/agenda.html`, `pages/patient_record.html` | Tailwind v4 só emite utilitários encontrados literalmente nos arquivos; `bg-{{x}}` nunca gera CSS. Hoje funciona porque o CSS é manual | Manter essas classes como **hooks semânticos** definidos em `@layer components` (`.status-concluido`, `.badge.st-agendado`), nunca como utilitários interpolados. Teste `web_test.go:82` depende de `status-concluido` |
| 2 | Tailwind exige Node no build, o projeto é Go puro | `Makefile`, `README.md`, `package.json` | Se o CSS gerado não for commitado, `go build` sem Node quebra o visual ou falha | CSS compilado é commitado e embutido (`//go:embed static`); `make css` regenera; fonte em `internal/web/styles/app.css` fora do embed; `assetVersion` já invalida cache pelo hash |
| 3 | Rótulos de navegação somem no celular (`.brand span, .sync span { display:none }` < 560px): botão só com ícone para quem é leiga | `app.css:62`, `layout.html` | Usuária não sabe o que o ícone de "sincronizar" faz; "Sincronizar" é jargão | Barra de abas inferior fixa com ícone + texto (Agenda · Pacientes · Ajuda); ação renomeada "Atualizar planilha" com descrição |
| 4 | Template compara literal `"concluido"` para esconder ações (CA-007 do revisor de arquitetura) | `partials/agenda.html:46,52` | Renomear a constante no domínio compila e volta a mostrar "Cancelar" em sessão concluída | `func (a Appointment) Done() bool` no domínio e `{{if not .Done}}` |

## Recommendations
| # | Issue | File(s) | Why | Fix |
|---|-------|---------|-----|-----|
| 1 | Nenhuma explicação de como o app conversa com Google Agenda/planilha | layout, connect, agenda | A cliente precisa entender por que um evento "marcado fora do app" aparece e o que "vincular" faz | Página `/ajuda` com diagrama SVG (celular ↔ Google ↔ app ↔ planilha), legenda de cores em `<details>` na agenda, caixa explicativa no card sem vínculo |
| 2 | Estados vazios só com texto curto | `agenda-day`, `patient-list` | Primeiro uso sem orientação | Ilustração SVG + frase do que fazer + botão da ação principal |
| 3 | Formulários sem dicas de preenchimento | `patient_form`, `appointment-form`, `evolutions` | Campos como "Endereço" têm efeito no Google (vira local do evento); usuária não sabe | `<p class="hint">` sob cada campo relevante; `aria-describedby` |
| 4 | Chips da semana truncados em "Fisio: M…" | `agenda-week` | Prefixo "Fisio: " consome o espaço útil | Função `shortTitle` remove o prefixo só na exibição; `title` mantém o completo |
| 5 | Frase dos lembretes hard-coded e falsa em demo (CA-008) | `appointment-form` | Política vive no adaptador do Calendar | Mostrar a frase só fora do modo demo (o partial recebe `Demo` via `dict`) |
| 6 | Dark mode só por `prefers-color-scheme`, sem override | `app.css` | Suficiente para uma usuária; evitar toggle (estado extra) | Manter automático: tokens semânticos em `@theme inline` apontando para variáveis que mudam no `@media` |
| 7 | Toast some em 3,5 s sem ação | `app.js` | Aviso de "planilha não atualizada" (MirrorWarning) pode passar despercebido | Avisos ficam também no `.notice` dentro do partial (já existe); toast só confirma |

## Nice-to-Haves
| # | Suggestion | Rationale |
|---|------------|-----------|
| 1 | Resumo do dia no topo da agenda ("3 atendimentos · 1 marcado fora do app") | Leitura rápida no celular antes de sair de casa |
| 2 | Botão "WhatsApp" com texto, não só o número | Deixa claro que abre uma conversa |
| 3 | Transições curtas (dialog, chips) respeitando `prefers-reduced-motion` | Sensação de resposta sem distrair |
| 4 | Conflito de horário sinalizado no card ("mesmo horário de …") | Dois 20:30 passaram sem aviso no E2E; fora do contrato atual, candidato a próxima iteração |

## Render Path Analysis
- **Navegação de dia/semana**: `hx-get` → servidor devolve `agenda-day`/`agenda-week` → `outerHTML` em `#agenda-main` + `hx-push-url`. Re-render é o bloco inteiro da agenda (dezenas de nós). Adequado: sem estado cliente a preservar, e o custo é uma requisição por clique.
- **Ações no card (concluir/cancelar/vincular)**: POST → mesmo partial inteiro. Poderia ser só o `<li>`, mas o resumo do dia e a legenda dependem da lista toda; manter o bloco inteiro evita estado parcial inconsistente.
- **Modal**: `hx-get` → `innerHTML` em `#modal-body` → `htmx:afterSwap` abre o `<dialog>`. POST do form mira `#modal-body`; em 422 o servidor re-renderiza o form no modal; em sucesso retargeta `#agenda-main` (HX-Retarget) e fecha o modal com `HX-Trigger-After-Swap` (ordem correta, corrigida na iteração 2). Nenhum listener acumula: `app.js` registra uma vez no `document`/`body`.
- **Busca de pacientes**: `input changed delay:300ms` → `outerHTML` em `#patient-list`. Debounce já evita requisições por tecla.
- **Renders desnecessários**: nenhum detectado; o único risco é CSS com `transition: all`, evitado (transições nomeadas).
- **Bundle**: htmx 51 KB + app.js 2 KB + CSS gerado estimado 20–35 KB minificado; tudo embutido, cache de 1 dia com hash na URL.

## Type Safety Assessment
Templates Go são dinâmicos: um campo inexistente em `.Data` só falha em execução. Mitigações presentes: structs de view nomeadas por handler (`agendaDayData`, `patientRecordData`…), `Renderer.Page` falha alto para página desconhecida, `web_test.go` renderiza todas as rotas em demo. Lacuna: partials chamados com `dict` perdem tipagem (`appointment-item`). Recomendação: manter `dict` só para esse caso e cobrir cada partial novo no teste de handlers (feito para `help`).

## Verdict Details
- Component Architecture: PASS (partials coesos; `appointment-item` reutilizado; ids estáveis como contrato)
- State Management: PASS (estado no servidor; DOM é a única fonte no cliente; modal/toast idempotentes)
- Type Safety: CONCERNS (templates dinâmicos; cobertos por testes de renderização)
- Rendering Performance: PASS (swaps por bloco, debounce na busca, sem listeners acumulados)
- Bundle Impact: PASS (assets embutidos, CSS gerado em build, sem CDN)

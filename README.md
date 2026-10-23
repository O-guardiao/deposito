# Fisio — agenda e fichas para fisioterapia domiciliar

Aplicativo web leve (Go + HTMX) para uma fisioterapeuta que atende em casa e
precisa parar de esquecer atendimentos. Ele **não tem banco de dados próprio**:

- **Google Agenda** é a agenda de verdade. Cada atendimento é um evento com
  lembretes (1 dia antes e 1 hora antes). O que ela marca no celular aparece no
  app; o que marca no app aparece no celular.
- **Google Planilhas** é a base de pacientes: ficha, evoluções (notas de sessão)
  e um espelho dos atendimentos, para ela pesquisar e conferir pelo navegador
  ou pelo app do Planilhas.

Cada paciente tem um **ID próprio** (`P-20261023-K7Q2M`), usado para ligar o
evento da agenda à ficha. Buscar o nome do paciente traz tudo: dados, próximos
atendimentos, histórico e evoluções.

```
 celular (Google Agenda / Planilhas)            computador (navegador)
        │  ▲                                           │  ▲
        ▼  │                                           ▼  │
 ┌──────────────────┐   leitura ao vivo       ┌─────────────────────┐
 │  Google Calendar │ ◄─────────────────────  │                     │
 │  (fonte da agenda)│ ───────────────────── ►│   fisio (Go + HTMX) │
 └──────────────────┘   cria/edita eventos    │                     │
 ┌──────────────────┐   pacientes, evoluções  │  sem banco próprio  │
 │  Google Sheets   │ ◄─────────────────────►│                     │
 │  (fichas + espelho)│                       └─────────────────────┘
 └──────────────────┘
```

## Como funciona a “mão dupla”

| Ação                                    | Google Agenda                              | Planilha                               |
|-----------------------------------------|--------------------------------------------|----------------------------------------|
| Agendar pelo app                        | cria evento `Fisio: <nome>` com lembretes  | linha na aba **Agendamentos**          |
| Remarcar / concluir / cancelar pelo app | atualiza / marca / remove o evento         | linha atualizada (cancelado fica registrado) |
| Marcar direto no celular                | evento comum                               | aparece no app como “marcado fora do app”; clique em **Vincular** para ligar ao paciente |
| Cadastrar paciente / evolução           | —                                          | abas **Pacientes** / **Evolucoes**     |
| Botão **Sincronizar**                   | lê os últimos 90 dias + próximos 180       | reescreve a aba **Agendamentos** (linhas `cancelado` são mantidas) |

O app grava **primeiro na agenda, depois na planilha**. Se a planilha falhar, o
atendimento não se perde: aparece um aviso e o botão Sincronizar corrige.

## Rodando em modo demo (sem Google)

```bash
make demo          # ou: go run ./cmd/fisio -demo
```

Abra <http://localhost:8080>. Os dados são fictícios e ficam só na memória; o
banner amarelo lembra disso. Serve para conhecer o app e para testes.

Por padrão o app escuta só em `127.0.0.1:8080` (a própria máquina). Para abrir
para outros aparelhos da rede use `-addr 0.0.0.0:8080`, sabendo que qualquer
um na rede passa a usar a conta Google conectada.

## Rodando de verdade (conta Google dela)

Precisa de Go 1.24+ e de um projeto no Google Cloud. É feito uma vez só.

### 1. Criar o projeto e ativar as APIs

1. Acesse <https://console.cloud.google.com/> com a conta Google dela e crie um
   projeto (ex.: `fisio`).
2. Em **APIs e serviços › Biblioteca**, ative **Google Calendar API** e
   **Google Sheets API**.

### 2. Tela de consentimento

1. **APIs e serviços › Tela de permissão OAuth** → tipo **Externo**, nome
   “Fisio”, e-mail de suporte dela. Salve.
2. Em **Usuários de teste**, adicione o e-mail dela. (Enquanto o app estiver em
   “teste”, só os e-mails listados conseguem entrar — perfeito para uso pessoal.)

### 3. Credenciais

1. **APIs e serviços › Credenciais › Criar credenciais › ID do cliente OAuth**.
2. Tipo **Aplicativo da Web**; em **URIs de redirecionamento autorizados**
   adicione exatamente `http://localhost:8080/auth/callback`.
3. Baixe o JSON e salve como `credentials.json` na pasta do app.

### 4. Subir o app e conectar

```bash
make run           # ou: go run ./cmd/fisio
```

Abra <http://localhost:8080> e clique em **Conectar com Google**. Autorize. Na
primeira vez o app cria a planilha **“Fisio - Pacientes”** com as abas certas e
guarda o ID em `data/fisio.json`; o token fica em `data/token.json` (com
permissão 600). Dali em diante não pede login de novo.

Para usar uma planilha já existente, informe `-spreadsheet <ID>`; as abas que
faltarem são criadas. Para usar uma agenda secundária em vez da principal,
`-calendar <id>` (o ID aparece em Configurações da agenda no Google).

### Opções

Cada opção pode ser passada como flag ou como variável de ambiente exportada no
shell (o app não lê arquivo `.env`; `.env.example` só lista os nomes).

| Flag / variável                        | Padrão                  | Para quê |
|----------------------------------------|-------------------------|----------|
| `-addr` / `FISIO_ADDR`                 | `127.0.0.1:8080`        | endereço HTTP (só a máquina dela) |
| `-demo` / `FISIO_DEMO=1`               | off                     | modo demo |
| `-data` / `FISIO_DATA`                 | `./data`                | token e configuração |
| `-credentials` / `FISIO_CREDENTIALS`   | `./credentials.json`    | credencial OAuth |
| `-token` / `FISIO_TOKEN`               | `<data>/token.json`     | onde o token OAuth fica salvo |
| `-spreadsheet` / `FISIO_SPREADSHEET_ID`| (salvo em data/fisio.json) | planilha |
| `-calendar` / `FISIO_CALENDAR_ID`      | `primary`               | agenda |
| `-tz` / `FISIO_TZ`                     | `America/Sao_Paulo`     | fuso dos dias |
| `-public-url` / `FISIO_PUBLIC_URL`     | `http://localhost:<porta>` | precisa bater com o redirect do Google |

Permissões pedidas ao Google: agenda (ler/editar eventos) e planilhas
(ler/editar). O app só mexe na planilha “Fisio - Pacientes” e nos eventos que
você agendar por ele; se o Google revogar o acesso, a tela “Conectar” volta a
aparecer.

## Estrutura da planilha

| Aba            | Colunas |
|----------------|---------|
| `Pacientes`    | ID, Nome, Telefone, Endereco, Nascimento (AAAA-MM-DD), Diagnostico, Observacoes, CriadoEm, AtualizadoEm |
| `Evolucoes`    | ID, PacienteID, Data, Texto, CriadoEm |
| `Agendamentos` | EventoID, PacienteID, Titulo, Inicio, Fim, Status (`agendado`/`concluido`/`cancelado`), Observacoes, Local, AtualizadoEm |

Ela pode editar as células pela planilha (ex.: corrigir um telefone); o app lê
tudo a cada acesso, do jeito que aparece na tela (a planilha é criada em
português, então datas digitadas como `25/12/1980` funcionam). Só não altere a
coluna **ID** nem a ordem das colunas.

Nos eventos da agenda, o app grava propriedades privadas (`fisio_patient_id`,
`fisio_status`) — invisíveis no celular, mas é assim que ele sabe de quem é o
evento.

## Estrutura do código

```
cmd/fisio                 main: flags, wiring, OAuth, shutdown
internal/domain           entidades (Patient, Evolution, Appointment) e portas (interfaces)
internal/domain/domaintest  testes de contrato que todo adaptador de porta deve passar
internal/app              casos de uso: PatientService, EvolutionService, AgendaService, SyncService
internal/adapters/memory  implementação em memória (demo e testes)
internal/adapters/ids     gerador de IDs de produção (P-AAAAMMDD-XXXXX)
internal/adapters/gcalendar  Google Calendar → domain.Agenda
internal/adapters/gsheets    Google Sheets → repositórios + espelho + bootstrap da planilha
internal/adapters/gauth      OAuth2 (login, token em disco, refresh)
internal/web              handlers HTTP, templates HTMX, CSS/JS (tudo embutido no binário)
```

Dependências: só `google.golang.org/api` e `golang.org/x/oauth2`. HTMX 2.0.4
está vendorado em `internal/web/static` (sem CDN).

```bash
make test   # testes unitários (serviços, mapeamentos, handlers)
make vet
```

## Limitações conhecidas

- **Uso individual.** Um usuário, uma conta Google. Sem login próprio: quem
  acessar a porta usa a conta conectada — por isso o padrão é escutar só em
  `127.0.0.1`. Formulários de outros sites são recusados (checagem de origem).
- **Sem cache.** Cada página consulta o Google. Para o volume de uma
  profissional é instantâneo; para dezenas de usuários não serviria.
- **Agenda principal.** Por padrão lê a agenda principal, então compromissos
  pessoais aparecem como “marcado fora do app” (útil para ver o dia ocupado).
  Use `-calendar` se preferir uma agenda só de trabalho.
- **Cancelar remove o evento** do Google (fica registrado na planilha como
  `cancelado`, e Sincronizar preserva essas linhas).
- **Sem aviso de conflito.** Dois atendimentos no mesmo horário são aceitos; a
  visão do dia mostra os dois.
- Sem lembrete por WhatsApp; o link no telefone do paciente abre o WhatsApp
  para ela mandar a mensagem.

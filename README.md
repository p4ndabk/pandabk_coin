# base-project-go

Projeto base em Go usando Gin + GORM, organizado por domínio (sem
interfaces/repository). Detalhes de arquitetura e convenções em
[CLAUDE.md](./CLAUDE.md).

> 🐼 **PANDA Coin**: este repositório também abriga o full node da PANDA
> (`cmd/node`) — binário estático, sem Gin/GORM, que valida a chain, fala
> p2p e **minera por padrão** (`CGO_ENABLED=0 go build -o bin/panda-node
> ./cmd/node && bin/panda-node run`). **Documentação oficial (instalação,
> multi-node, Tor, referência de comandos): [docs/README.md](./docs/README.md)**.
> Guia narrado para iniciantes: [TUTORIAL.md](./TUTORIAL.md). Plano e decisões
> de design em [PLAN.md](./PLAN.md); visão em [PROPOSTA.md](./PROPOSTA.md).

## Requisitos

- Go 1.25+

## Configuração

Copie o `.env.example` para `.env` e ajuste se precisar:

```bash
cp .env.example .env
```

Variáveis disponíveis:

| Variável    | Descrição                                    | Default                 |
|-------------|-----------------------------------------------|--------------------------|
| `PORT`      | Porta HTTP do servidor                         | `8080`                   |
| `DB_DRIVER` | `sqlite` ou `postgres`                         | `sqlite`                 |
| `DB_PATH`   | Caminho do arquivo SQLite (só com `DB_DRIVER=sqlite`) | `data/base_project.db` |
| `DB_DSN`    | Connection string do Postgres (só com `DB_DRIVER=postgres`), ex. `host=localhost user=postgres password=postgres dbname=base_project port=5432 sslmode=disable` | vazio |
| `JWT_SECRET` | Chave usada para assinar/validar o JWT | `dev-secret-change-me` |
| `CORS_ALLOWED_ORIGINS` | `*` (qualquer origem) ou lista separada por vírgula, ex. `https://example.com,https://admin.example.com` | `*` |

## Rodando o projeto

```bash
go mod download
go run ./cmd/migrate   # cria a pasta data/ (se não existir) e roda as migrations pendentes
go run ./cmd/api        # sobe o servidor
```

O servidor sobe em `http://localhost:8080` (ou na porta definida em `PORT`).
O `cmd/api` não migra o banco sozinho — migração é responsabilidade do
`cmd/migrate`, rodado à parte (assim como no Artisan do Laravel). O arquivo
do banco não é versionado (veja `.gitignore`).

## Migrations

`cmd/migrate` usa [`gormigrate`](https://github.com/go-gormigrate/gormigrate):
cada migration fica em `internal/database/migrations/migrations.go`, e o
gormigrate guarda numa tabela `migrations` quais IDs já rodaram — rodar o
comando de novo só aplica o que ainda falta (idempotente, sem apagar dado
existente).

```bash
go run ./cmd/migrate            # aplica as migrations pendentes
go run ./cmd/migrate -rollback  # desfaz só a última migration aplicada
```

## Comandos (`cmd/`)

| Comando                        | O que faz                                                        |
|---------------------------------|--------------------------------------------------------------------|
| `go run ./cmd/api`              | Sobe o servidor HTTP                                                |
| `go run ./cmd/migrate`          | Aplica as migrations pendentes (ver seção Migrations acima)         |
| `go run ./cmd/migrate -rollback`| Desfaz a última migration aplicada                                  |
| `go run ./cmd/seed`             | Popula dados de exemplo (cria um usuário admin se ainda não existir) |

## Documentação da API (Swagger)

Com o servidor rodando, a Swagger UI fica disponível em
`http://localhost:8080/api/docs/index.html`.

A doc é gerada a partir de anotações nos `handler.go` de cada domínio (ver
`internal/docs/SPEC.md`). Sempre que adicionar/alterar uma rota, regenere:

```bash
go tool swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

As flags `--parseDependency --parseInternal` são necessárias porque `User`
embute `gorm.Model` (tipo de uma dependência externa). O resultado (pasta
`docs/`) é versionado — não precisa instalar o `swag` globalmente, ele já
está disponível via `go tool swag ...` (declarado como tool dependency no
`go.mod`).

## Tratamento de erros

Toda resposta de erro da API segue o mesmo envelope:

```json
{"error": {"code": "invalid_credentials", "message": "invalid credentials"}}
```

Erros inesperados (500) nunca expõem a mensagem real ao cliente — ela vai
pro log do servidor e o cliente recebe `{"error":{"code":"internal_error",
"message":"internal server error"}}`. Ver `internal/apierror` e a seção
"Error handling" do `CLAUDE.md` para a convenção completa.

## Testes

```bash
go test ./...
```

## Build

```bash
go build -o bin/app ./cmd/api
./bin/app
```

## Docker

`docker-compose.yml` sobe a API contra um Postgres real (via `DB_DRIVER=postgres`),
sem precisar de Go instalado na máquina:

```bash
docker compose up --build
```

Isso sobe o `postgres`, roda o `migrate` uma vez (serviço que sai após migrar)
e só então sobe a `api` em `http://localhost:8080`. O `Dockerfile` é
multi-stage: builda `api`, `migrate` e `seed` como binários estáticos
(`CGO_ENABLED=0`) e a imagem final só tem os binários + `ca-certificates`.

Rodar o seed dentro do compose:

```bash
docker compose run --rm migrate ./seed
```

`docker compose down -v` remove também o volume do Postgres.

## Gerar um novo projeto a partir deste base

O `cmd/create-project` copia esta estrutura (sem `.git`, `data/`, `bin/`,
`.env`, `cmd/create-project` etc.) para um novo diretório e já ajusta o
module path em todos os arquivos `.go` e no `go.mod`, rodando `go mod tidy`
no final. O próprio `cmd/create-project` não vai para o projeto gerado — é
ferramenta do template, não do app final.

```bash
go run ./cmd/create-project
```

Ele pergunta interativamente pelo nome do projeto (module path, ex.
`github.com/usuario/meu-app`) e pelo diretório de destino (ex. `../meu-app`).
Também dá pra pular os prompts passando as flags direto:

```bash
go run ./cmd/create-project -name github.com/usuario/meu-app -dir ../meu-app
```

Precisa ser rodado a partir da raiz deste repositório (onde está o `go.mod`
do base project).
# pandabk_coin

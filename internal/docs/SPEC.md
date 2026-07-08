# Spec: docs (Swagger/OpenAPI)

## Decisões & porquês (regra e arquitetura)

- **Doc gerada de anotações no próprio handler, não escrita à mão.** Um arquivo
  OpenAPI mantido à parte diverge do código no primeiro PR que ninguém lembra de
  atualizar. Anotar cada handler (`@Summary`/`@Router`/...) mantém a fonte da
  verdade *ao lado* da rota — quem muda a rota vê a anotação. É por isso que a
  convenção (todo endpoint anotado + `swag init` antes do PR) está no CLAUDE.md.
- **`docs/` gerado é comitado; `swag` é tool dependency do `go.mod`.** Comitar o
  pacote gerado evita exigir que todo dev instale o `swag` globalmente
  (versões divergentes gerariam docs diferentes) e mantém o build reprodutível —
  roda via `go tool swag`. As flags `--parseDependency --parseInternal` são
  necessárias porque `User` embute `gorm.Model`, tipo de uma dependência externa.
- **Sem `service.go` — só `routes.go` montando handler de terceiros.** Mesma
  exceção de infra do `health`: `docs` não é domínio de negócio, só registra o
  `ginSwagger.WrapHandler`. Nada a testar em unidade além do wiring.
- **Sem auth na UI, sem validação automática doc-vs-rota.** É um base project de
  dev: proteger a Swagger UI ou construir um teste que compara `@Router` com a
  rota real seria complexidade que o escopo não pede. A divergência é pega em
  revisão de PR — decisão consciente de deixar barato.

## Objetivo

Gerar documentação OpenAPI/Swagger a partir de anotações nos handlers de cada
domínio e expor uma UI interativa para explorar e testar as rotas da API.

## Escopo

O que **entra**:
- Anotações Swagger (formato `swaggo/swag`) em cada método de `handler.go`
  já existente (`health.Check`, `user.Create/List/Get/Update/Delete/Login/Me`)
  e regra de que todo novo endpoint criado a partir daqui vem anotado.
- Anotação geral da API (título, versão, descrição, host, basePath) em
  `cmd/api/main.go`.
- Pacote `internal/docs` responsável por registrar a rota que serve a
  Swagger UI (via `gin-swagger` + `swaggo/files`), lendo a spec gerada em
  `docs/swagger.json`/`docs/docs.go`.
- Comando para gerar/regenerar a doc (`swag init`) documentado no README.

O que **fica de fora** (evita scope creep):
- Autenticação/proteção da própria Swagger UI (fica pública, é ambiente de
  dev/base project).
- Exemplos de payload múltiplos ou customizados (`@example` avançado) — só
  `@Param`/`@Success`/`@Failure` básicos.
- Versionamento de múltiplas versões de API (v1/v2) na doc.
- Teste automatizado que valida se a doc bate com a rota real — é revisão
  manual em PR.

## Modelo de dados

N/A — não introduz model nem tabela nova.

## Regras de negócio

N/A — sem `service.go`. `internal/docs` só tem `routes.go`, registrando o
handler de terceiros (`ginSwagger.WrapHandler`) que serve os arquivos gerados
pelo `swag init`. Mesma exceção já aplicada a `internal/health`: infra, não
domínio de negócio.

## Endpoints

### `GET /api/docs/*any`
- **Request:** nenhum
- **Response (sucesso):** `200` + Swagger UI (HTML/JS) servindo a spec
  gerada em `docs/swagger.json`
- **Response (erro):** `404` se `docs/` não foi gerado (`swag init` não
  rodou ainda) — o pacote `docs` gerado não existiria e o build falharia
  antes de chegar a rodar

## Casos de erro / edge cases

- `swag init` não rodado após criar/alterar anotações → doc desatualizada ou
  build quebrado (import de `_ ".../docs"` em `main.go` depende do pacote
  gerado existir). Documentar no README como parte do fluxo de
  desenvolvimento (rodar `swag init` sempre que mudar uma anotação).
- Anotação de rota (`@Router`) divergente da rota real registrada em
  `routes.go` → doc mentirosa. Não há checagem automática; revisão manual.
- Novo domínio criado sem anotações nos handlers → doc incompleta
  silenciosamente (não quebra build). Vira item do checklist de "Critérios
  de aceite" no `BASE_SPEC.md` de cada novo domínio.

## Critérios de aceite

- [x] `github.com/swaggo/swag` (tool dependency), `github.com/swaggo/gin-swagger`,
      `github.com/swaggo/files` adicionados como dependência
- [x] Anotação geral da API (`@title`, `@version`, `@description`,
      `@BasePath`, `@securityDefinitions.apikey BearerAuth`) em
      `cmd/api/main.go`
- [x] Cada handler existente anotado: `health.Check`;
      `user.Create/List/Get/Update/Delete/Login/Me`
- [x] `internal/docs/routes.go` com `RegisterRoutes(rg *gin.RouterGroup)`
      montando `GET /docs/*any` via `ginSwagger.WrapHandler`, wired em
      `cmd/api/main.go`
- [x] `docs/` gerado via
      `go tool swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal`
      e comitado (evita exigir `swag` instalado globalmente — é tool
      dependency do `go.mod`; as flags são necessárias porque `User` embute
      `gorm.Model`, tipo de uma dependência externa)
- [x] README atualizado: comando para (re)gerar a doc e URL da Swagger UI
- [x] `CLAUDE.md` atualizado com a convenção: todo novo endpoint em
      `handler.go` deve vir com anotações Swagger, e `swag init` deve rodar
      antes de abrir o PR

## Fora de escopo / não fazer

- Sem autenticação na Swagger UI.
- Sem múltiplas versões de doc (v1/v2).
- Sem validação automatizada de doc-vs-rota — revisão manual.
- Sem anotações de exemplos avançados de payload.

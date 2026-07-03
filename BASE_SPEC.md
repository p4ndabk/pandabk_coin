# Spec: <nome do domínio/tarefa>

> Modelo de spec para uma nova feature/domínio. Copie este arquivo para
> `internal/<domínio>/SPEC.md` e preencha antes de implementar. Se uma seção
> não se aplica, deixe explícito "N/A" em vez de apagar — isso evita spec
> incompleta por omissão.

## Objetivo

O que essa feature resolve e por quê. 1-2 frases. Sem detalhe técnico aqui.

## Escopo

O que **entra** nessa tarefa:
-

O que **fica de fora** (evita scope creep — se não está listado aqui, não
implementa sem voltar e atualizar a spec):
-

## Modelo de dados

Campos do `model.go`, tipos e constraints (obrigatório, único, default,
relacionamento). Só o que já existe hoje — não adiciona campo especulando
uso futuro.

| Campo | Tipo | Constraint | Observação |
|-------|------|------------|------------|
|       |      |            |            |

## Regras de negócio

O que vai em `service.go`. Validações, cálculos, condições que não são só
"salvar no banco". Se for puro CRUD sem regra nenhuma, escreva "N/A — CRUD
simples".

-

## Endpoints

O que vai em `handler.go` + `routes.go`. Um bloco por endpoint.

### `<MÉTODO> /api/<caminho>`

- **Request:** corpo/params esperados
- **Response (sucesso):** status code + shape do JSON
- **Response (erro):** casos de erro esperados + status code. Erros seguem
  o envelope compartilhado `apierror.Body` (`{"error":{"code","message"}}`)
  — ver seção "Error handling" do CLAUDE.md. Nunca devolver `err.Error()`
  cru pro cliente num 500.

## Casos de erro / edge cases

Situações que o service/handler precisam tratar explicitamente (ex.: email
duplicado, registro não encontrado, usuário inativo).

-

## Critérios de aceite

- [ ] `model.go`, `service.go`, `handler.go`, `routes.go` criados seguindo o
      padrão de `internal/<domínio>/` (ver CLAUDE.md)
- [ ] `service_test.go` cobrindo as regras de negócio e os edge cases acima
      (obrigatório — ver seção Testing do CLAUDE.md)
- [ ] Rotas registradas em `cmd/api/main.go`
- [ ] Se o domínio tem tabela nova: migration adicionada em
      `internal/database/migrations/migrations.go` (ver seção Migrations
      do CLAUDE.md — nunca editar uma migration que já rodou, sempre somar
      uma nova)
- [ ] README/CLAUDE.md atualizados se a convenção mudou

## Fora de escopo / não fazer

Coisas que alguém pode ser tentado a adicionar mas que essa tarefa
explicitamente não cobre (ex.: "sem paginação nessa versão", "sem
autorização por papel/role ainda").

-

# Spec: health

## Decisões & porquês (regra e arquitetura)

- **Sem `model.go`/`service.go` — o handler fala direto com `*gorm.DB`.** Health
  é uma checagem de *infraestrutura*, não um domínio de negócio: não tem estado,
  regra nem tabela. Forçar a estrutura model/service/handler aqui criaria camadas
  vazias só para satisfazer um padrão. É a exceção explícita do CLAUDE.md,
  aplicada onde o custo do padrão não compra nada.
- **Banco indisponível retorna `503`, nunca `500`.** Um health check é lido por
  máquina (load balancer, monitor); "banco fora" é um *estado esperado* que o
  monitor precisa distinguir de "código quebrou". `503` (serviço indisponível) é
  a resposta semântica que faz o balanceador tirar a instância da rotação; `500`
  confundiria uma coisa com a outra.
- **Endpoint público, sem autenticação.** Health checks de infra rodam antes de
  qualquer credencial; exigir auth aqui quebraria justamente quem precisa
  consultá-lo. Como só devolve up/down (nenhum dado sensível), o custo de
  expô-lo é nulo.

## Objetivo

Expor um endpoint de infraestrutura para checar se a API está de pé e se a
conexão com o banco está saudável (usado por load balancer/monitoramento).

## Escopo

O que **entra**:
- Endpoint único que verifica conectividade com o banco via ping.

O que **fica de fora**:
- Checagem de outras dependências externas (fila, cache, storage) — não
  existem ainda neste projeto.
- Métricas/latência detalhada — só status up/down.

## Modelo de dados

N/A — não é um domínio de negócio, não tem model/tabela.

## Regras de negócio

N/A — sem `service.go`. O handler fala direto com `*gorm.DB` (exceção
documentada em CLAUDE.md).

## Endpoints

### `GET /api/health`

- **Request:** sem parâmetros
- **Response (sucesso):** `200` — `{"status":"ok","database":"up"}`
- **Response (erro):** `503` — `{"status":"error","database":"down"}` quando
  o ping no banco falha

## Casos de erro / edge cases

- Conexão com o banco fechada/indisponível → `503`, nunca `500` (é um estado
  esperado de monitoramento, não uma falha de código).

## Critérios de aceite

- [x] `handler.go`, `routes.go` criados (sem `model.go`/`service.go` — ver
      exceção no CLAUDE.md)
- [x] `handler_test.go` cobrindo banco up e banco down
- [x] Rota registrada em `cmd/api/main.go`
- [ ] Model no `cmd/migrate` — N/A, sem tabela
- [x] CLAUDE.md documenta a exceção deste domínio

## Fora de escopo / não fazer

- Sem autenticação nesse endpoint — precisa ser público para health checks
  de infraestrutura.

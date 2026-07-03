# Spec: user

## Objetivo

CRUD de usuários com autenticação básica por email/senha, servindo de base
para qualquer feature que precise identificar quem está fazendo a
requisição.

## Escopo

O que **entra**:
- CRUD completo de usuário (name, email, password, active).
- Login validando email + senha, retornando um JWT.
- Middleware `AuthRequired` para proteger rotas via `Authorization: Bearer
  <token>`, com `GET /api/me` como exemplo de referência de rota protegida.

O que **fica de fora** (implementar em spec própria quando necessário):
- Middleware de autorização por papel/role (hoje só autentica, não autoriza
  por permissão).
- Refresh token / revogação de token.
- Reset/recuperação de senha.
- Paginação no `List`.

## Modelo de dados

| Campo    | Tipo   | Constraint              | Observação                          |
|----------|--------|--------------------------|--------------------------------------|
| Name     | string | not null                 |                                       |
| Email    | string | not null, unique index   |                                       |
| Password | string | not null                 | armazenado com hash bcrypt, `json:"-"` (nunca serializado) |
| Active   | bool   | —                        | sem default no GORM de propósito — bool + `gorm:"default:true"` faz o GORM omitir `false` no insert e usar o default do banco (ver histórico de bug corrigido); default `true` é aplicado no handler quando `active` não vem no request |

## Regras de negócio (`service.go`)

- `Create`: gera hash bcrypt da senha antes de salvar.
- `Update`: só re-hasheia a senha se uma nova senha for enviada (string não
  vazia); caso contrário preserva o hash existente.
- `Authenticate(email, password)`: busca por email, rejeita se usuário
  inativo, compara hash com bcrypt. Qualquer falha (email não encontrado,
  inativo, senha errada) retorna o mesmo erro genérico `ErrInvalidCredentials`
  — não vazar qual dessas condições falhou.
- `Create`/`Update`: email duplicado (unique constraint) retorna o sentinel
  `ErrEmailTaken` em vez de deixar o erro de banco subir cru (ver
  `apierror.Conflict` no handler).

## Endpoints

### `POST /api/users`
- **Request:** `{"name","email","password" (min 6), "active"? (default true)}`
- **Response (sucesso):** `201` + usuário criado (sem password)
- **Response (erro):** `400` validação (`validation_error`); `409` email
  duplicado (`email_taken`); `500` erro inesperado — todo erro no formato
  `apierror.Body`

### `GET /api/users`
- **Response (sucesso):** `200` + lista de usuários

### `GET /api/users/:id`
- **Response (sucesso):** `200` + usuário
- **Response (erro):** `400` id inválido; `404` não encontrado

### `PUT /api/users/:id`
- **Request:** `{"name","email","password"? ,"active"?}`
- **Response (sucesso):** `200` + usuário atualizado
- **Response (erro):** `400` validação/id inválido; `404` não encontrado

### `DELETE /api/users/:id`
- **Response (sucesso):** `204`
- **Response (erro):** `400` id inválido

### `POST /api/login`
- **Request:** `{"email","password"}`
- **Response (sucesso):** `200` + `{"token","user"}` (JWT HS256, 24h de validade)
- **Response (erro):** `401` credenciais inválidas (`invalid_credentials`,
  não distingue motivo)

### `GET /api/me`
- Protegida por `AuthRequired` (header `Authorization: Bearer <token>`).
  Serve de exemplo de referência para proteger qualquer rota futura.
- **Response (sucesso):** `200` + usuário autenticado (resolvido pelo id do
  token)
- **Response (erro):** `401` sem token/token inválido/expirado
  (`unauthorized`); `404` usuário do token não existe mais

## Casos de erro / edge cases

- Email duplicado no `Create`/`Update` → `409` (`email_taken`), não `500`.
- Usuário inativo tentando logar → mesmo erro genérico de credencial
  inválida, não um erro diferenciado.
- `Update` sem senha no payload não deve apagar a senha existente.
- Token ausente, malformado ou expirado em rota protegida → `401`
  genérico, sem distinguir o motivo pro cliente.

## Critérios de aceite

- [x] `model.go`, `service.go`, `handler.go`, `routes.go` criados
- [x] `service_test.go` cobrindo create/list/get/update/delete/authenticate
      (sucesso, senha errada, inativo, email desconhecido, email duplicado)
- [x] Rotas registradas em `cmd/api/main.go`
- [x] Migration `20260702000001_create_users_table` em
      `internal/database/migrations/migrations.go` (gormigrate)
- [x] Seed de usuário admin em `cmd/seed`
- [x] JWT emitido no login (`token.go`) e middleware `AuthRequired`
      (`middleware.go`) protegendo `GET /api/me`
- [x] Respostas de erro no formato `apierror.Body` (ver CLAUDE.md → Error
      handling)

## Fora de escopo / não fazer

- Sem paginação em `List`.
- Sem autorização por papel/role — só autenticação.
- Sem refresh token / revogação de token.

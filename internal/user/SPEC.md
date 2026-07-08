# Spec: user

## Decisões & porquês (regra e arquitetura)

Este é o domínio de referência do skeleton — a forma que todo novo domínio deve
copiar. As decisões são sobre segurança de credencial e sobre não vazar
informação por acidente.

- **Senha em bcrypt, nunca em claro; `json:"-"` no campo.** bcrypt tem custo
  ajustável (resiste a força bruta) e salt embutido. Marcar o campo como
  `json:"-"` garante que o hash nunca sai numa resposta, mesmo que alguém
  serialize o `User` inteiro por engano. Duas camadas para o mesmo erro caro.
- **`Authenticate` devolve *um* erro genérico para email inexistente, senha
  errada e conta inativa.** Distinguir "email não existe" de "senha errada"
  entrega a um atacante um oráculo de enumeração de contas. `ErrInvalidCredentials`
  único fecha esse vazamento — a UX levemente pior é o preço certo pela segurança.
- **Unique constraint traduzida para `ErrEmailTaken` no service, `409` no
  handler.** Deixar o erro cru do banco subir viraria um `500` genérico (e poderia
  vazar detalhe de driver/SQL). Traduzir para um sentinel no service e mapear para
  `apierror.Conflict` no handler mantém a fronteira do CLAUDE.md: service
  HTTP-agnóstico, handler dono do status. Exige `TranslateError: true` no GORM.
- **`Update` só re-hasheia a senha se vier uma nova.** Um PUT que não menciona
  senha não deve apagá-la. Checar string não-vazia antes de re-hashear preserva o
  hash existente — evita o bug clássico de "editei o nome e perdi a senha".
- **`Active` sem `gorm:"default:true"`, default aplicado no handler.** Documentado
  inline por causa de um bug real: `bool + default:true` faz o GORM omitir `false`
  no insert e usar o default do banco, tornando impossível criar um usuário
  inativo. Aplicar o default no handler quando `active` não vem no request é o que
  torna os dois estados representáveis.
- **JWT HS256 stateless, 24h, sem refresh/revogação.** Um token assinado que o
  servidor valida sem consultar o banco é o mínimo viável de auth. Refresh,
  revogação e roles são features de spec própria quando o projeto precisar —
  adicioná-las agora seria infra especulativa. `GET /api/me` existe como o
  exemplo de referência de rota protegida a copiar.

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

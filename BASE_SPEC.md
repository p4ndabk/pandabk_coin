# Spec: <nome do pacote/tarefa>

> Modelo de spec para um novo pacote do node Zhu. Copie este arquivo para
> `internal/<domínio>/SPEC.md` e preencha antes de implementar. Se uma seção
> não se aplica, deixe explícito "N/A" em vez de apagar — isso evita spec
> incompleta por omissão. Veja `internal/p2p/SPEC.md` ou
> `internal/chain/SPEC.md` como exemplo de como fica depois de adaptado.

## Conceito

Explicação didática do conceito de blockchain/rede que esse pacote
implementa — o leitor aprende o domínio pelo próprio código. 1-2 parágrafos,
sem jargão de implementação ainda.

## Decisões & porquês (regra e arquitetura)

As decisões de design não óbvias deste pacote e o motivo de cada uma —
trade-offs considerados, por que a alternativa mais simples não serviu.

-

## Objetivo

O que esse pacote resolve e por quê. 1-2 frases. Sem detalhe técnico aqui.

## Escopo

O que **entra** nessa tarefa:
-

O que **fica de fora** (evita scope creep — se não está listado aqui, não
implementa sem voltar e atualizar a spec):
-

## Modelo de dados

Structs/tipos principais, campos e invariantes. Só o que já existe hoje —
não adiciona campo especulando uso futuro.

| Campo | Tipo | Observação |
|-------|------|------------|
|       |      |            |

## Regras de negócio

Validações, cálculos, condições que o pacote impõe — o que vai no
`service.go`/lógica principal do pacote.

-

## Interface do pacote / CLI

Funções exportadas, subcomandos de CLI ou métodos de RPC que este pacote
expõe. Um bloco por função/comando relevante.

```go
func Exemplo(...) (...)
```

## Casos de erro / edge cases

Situações que o pacote precisa tratar explicitamente (ex.: endereço
inválido, estado inconsistente, timeout de rede).

-

## Critérios de aceite

- [ ] Arquivos do pacote criados seguindo o padrão de `internal/<domínio>/`
      (ver CLAUDE.md)
- [ ] `service_test.go`-equivalente cobrindo as regras de negócio e os
      edge cases acima (obrigatório)
- [ ] `go test -race` verde
- [ ] SPEC.md deste pacote atualizado se a convenção mudou

## Fora de escopo / não fazer

Coisas que alguém pode ser tentado a adicionar mas que essa tarefa
explicitamente não cobre.

-

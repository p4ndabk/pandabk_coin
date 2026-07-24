# ZEN-NNN: <título curto da regra>

`draft` · `<consenso|rede|política|interoperabilidade|interface-local>`

## Resumo

Uma ou duas frases: o que esta regra é, em termos que alguém que nunca leu o
código Go entende.

## Motivação

Por que essa regra existe — o problema que ela resolve ou o ataque que
fecha. Se troca um trade-off (ex.: simplicidade por robustez), diga qual e
por que este lado foi escolhido.

## Especificação

A regra em si, normativa e completa: formatos de bytes, faixas de valores,
algoritmos, ordem de operações. Detalhe o bastante para duas implementações
independentes (em linguagens diferentes) produzirem exatamente os mesmos
bytes/decisões. Use tabelas para campos de estrutura e listas numeradas para
sequências de validação.

## Casos de erro

Entradas malformadas ou de fronteira e o que uma implementação conforme deve
fazer com cada uma.

## Compatibilidade

O que quebra (ou não) para nodes que já seguem a regra anterior — hard fork,
soft fork, ou aditivo sem impacto.

## Referência de implementação

Pacote(s) Go e arquivo(s) que implementam esta regra hoje (se já
implementado) — ex. `internal/core/block.go`.

## Ver também

Links para outros ZENs relacionados.

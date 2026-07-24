# ZEN — Zhu Enhancement Notes

Assim como o Nostr documenta seu protocolo em [NIPs](https://github.com/nostr-protocol/nips)
(Nostr Implementation Possibilities), a Zhu documenta as regras que **precisam
ser as mesmas em todo node da rede** em ZENs — Zhu Enhancement Notes. Um ZEN
não é um tutorial nem um SPEC.md de pacote: um `SPEC.md` (ver
[BASE_SPEC.md](../../BASE_SPEC.md)) documenta a implementação de um pacote
Go específico para quem mexe no código; um ZEN documenta uma **regra de
protocolo**, para qualquer pessoa que queira reimplementar um node Zhu do
zero, em qualquer linguagem, e continuar compatível com a rede.

Regra prática: se mudar a regra faz dois nós discordarem sobre o estado da
cadeia (bloco válido, endereço certo, ponta certa, mensagem entendida), é
consenso e pertence a um ZEN. Se é só sobre como este binário Go específico
está organizado por dentro, é `SPEC.md`.

## Índice

| ZEN | Título | Status | Categoria | Pacote(s) fonte |
|-----|--------|--------|-----------|------------------|
| [ZEN-001](./ZEN-001.md) | Formato canônico de bloco e transação | `final` | consenso | `core` |
| [ZEN-002](./ZEN-002.md) | Proof of Work Argon2id — target e retarget | `final` | consenso | `pow` |
| [ZEN-003](./ZEN-003.md) | Validação de bloco e fork choice | `final` | consenso | `chain` |
| [ZEN-004](./ZEN-004.md) | Protocolo de rede P2P | `final` | rede | `p2p` |
| [ZEN-005](./ZEN-005.md) | Parâmetros de consenso e política monetária | `final` | consenso | `params` |
| [ZEN-006](./ZEN-006.md) | Política de mempool e relay de transações | `final` | política | `mempool` |
| [ZEN-007](./ZEN-007.md) | Backup mnemônico (BIP39) e derivação de chave (SLIP-0010) | `final` | interoperabilidade | `wallet` |
| [ZEN-008](./ZEN-008.md) | Interface RPC JSON do node (controle local) | `final` | interface-local | `node` |

Categorias, da mais para a menos crítica em termos de "quebra a rede se
divergir":

- **consenso** — divergir causa split de rede (dois nodes discordam sobre o
  estado da cadeia). Mudar exige hard fork / rede nova.
- **rede** — precisa ser igual para dois nodes se conectarem e trocarem
  dados, mas não decide qual bloco é válido; extensões são geralmente
  aditivas (tipo de mensagem desconhecido é ignorado, ZEN-004).
- **política** — comportamento local de cada node (o que aceita esperar no
  mempool) que pode divergir entre implementações sem separar a rede.
- **interoperabilidade** — formato compartilhado com padrões externos
  (BIP39/SLIP-0010) para portar segredos entre implementações, sem relação
  com o P2P.
- **interface-local** — contrato entre o node e um cliente de controle
  (CLI, wallet gráfica) rodando na mesma máquina; não trafega na rede P2P.

## Status

- **`draft`** — em discussão, ainda pode mudar de forma incompatível.
- **`final`** — implementado e em uso; mudar exige um novo ZEN que o
  substitua (nunca um edit silencioso, porque nós já rodam essa regra).
- **`deprecated`** — regra antiga documentada por histórico; não seguir em
  implementações novas.

Os cinco ZENs acima descrevem o protocolo tal como implementado hoje — são
`final` porque a rede devnet já roda essas regras. Um ZEN novo nasce
**`draft`**: propõe uma mudança ou extensão (ex.: compact blocks, ban score,
parâmetros de mainnet — ver as seções "Fora de escopo" dos `SPEC.md` de cada
pacote para candidatos) antes de virar código.

## Como propor um ZEN

1. Copie o [template](./TEMPLATE.md) para `docs/zen/ZEN-NNN.md` (próximo
   número livre, sequencial).
2. Preencha Resumo, Motivação, Especificação e Compatibilidade — a
   especificação precisa ser detalhada o bastante para alguém reimplementar
   sem ler o código Go.
3. Abra a mudança com status `draft`.
4. Só depois de implementado, testado (`go test -race ./...`) e rodando em
   devnet, o ZEN muda para `final` — nesse ponto ele é regra de consenso, e
   revisar o texto exige tanto cuidado quanto revisar o código que o
   implementa.

## Ver também

- [BASE_SPEC.md](../../BASE_SPEC.md) — template de spec por pacote (visão
  interna, não de protocolo)
- [PLAN.md](../../PLAN.md) — decisões de design e roadmap do projeto
- [CLAUDE.md](../../CLAUDE.md) — arquitetura e convenções do repositório

# Spec: p2p — protocolo de rede, peers e sincronização

> Domínio do node PANDA (ver [PLAN.md](../../PLAN.md)). Depende de `core`,
> `chain`, `mempool`, `params`. TCP puro + stdlib — sem libp2p.

## Conceito

A **descentralização** da rede é literalmente este pacote: não existe servidor
central — cada nó conversa com um punhado de **peers** (vizinhos), e
informação se espalha por **fofoca (gossip)**: eu te conto os blocos e
transações que conheço, você conta aos seus vizinhos, e em segundos a rede
inteira sabe. Derrubar a rede exigiria derrubar todos os nós ao mesmo tempo.

Mecânicas centrais:

- **Handshake**: ao conectar, os nós trocam uma mensagem `version` com o hash
  do bloco gênesis (se difere, são redes diferentes — desconecta), a altura e
  o trabalho acumulado de cada um. Assim cada lado sabe se está atrasado.
- **Gossip por inventário**: em vez de empurrar blocos inteiros, um nó anuncia
  `inv` ("tenho o bloco X / a tx Y"); quem não tem pede com `getdata`. Evita
  tráfego duplicado.
- **Initial Block Download (IBD)**: um nó novo (ou desatualizado) pede a
  história por partes — primeiro os **headers** (96 bytes cada, baratos) via
  `getheaders` com um *locator* (lista de hashes espaçados exponencialmente a
  partir da ponta, para achar o ponto em comum), depois os corpos dos blocos
  em janelas. Valida e conecta em ordem.
- **Peer exchange**: peers trocam endereços de outros peers (`getaddr`/`addr`),
  então basta conhecer um nó para descobrir a rede.

## Objetivo

Conectar nós entre si, propagar blocos e transações, e sincronizar a cadeia de
qualquer nó novo até a ponta — sem coordenador central.

## Escopo

Entra:
- `message.go` — envelope `{Type string, Payload json.RawMessage}`; tipos:
  `version`, `verack`, `ping`, `pong`, `getaddr`, `addr`, `inv`, `getdata`,
  `getheaders`, `headers`, `block`, `tx`, `reject`. Blocos/txs viajam como
  bytes canônicos base64 dentro do JSON
- `codec.go` — frame: 4 bytes big-endian de tamanho + JSON; frame máx 1 MiB;
  funciona sobre `io.ReadWriter` (testável com `net.Pipe`)
- `peer.go` — loop por peer, estado do handshake, ping/pong a cada 2 min
  (drop após 2 falhas), detecção de auto-conexão por nonce
- `server.go` — listener TCP, dial dos seeds (`--peers` primeiro), peer
  manager (máx 8 outbound), address book persistido no bucket `meta` da chain,
  gossip de `addr` (até 32 endereços)
- `sync.go` — IBD: se cumWork do peer > nosso → `getheaders` com locator,
  receber até 2.000 headers (validar encadeamento + bits; Argon2 só no bloco
  completo), pedir corpos via `getdata` em janelas de 16, conectar em ordem;
  steady-state: `inv` → `getdata` → `block`/`tx`

**Node caseiro atrás de NAT (princípio "um node em cada casa" do PLAN.md):**
um node que só disca para fora (outbound-only) é cidadão pleno da rede —
valida tudo, minera e propaga blocos/txs pelas conexões que ele mesmo abriu.
Aceitar conexões de entrada (exige port forward no roteador) é opcional e
apenas aumenta a capilaridade da rede. O design não pode assumir que peers são
alcançáveis de fora: o address book só anuncia `listen_addr` de quem declarou
aceitar entrada.

Fica de fora:
- NAT traversal automático / UPnP (roadmap prioritário pós-v1 — outbound-only
  já cobre o node doméstico; traversal só melhora a topologia)
- Criptografia de transporte (rede de dev; anotar como evolução futura)

## Modelo de dados (mensagem version)

| Campo | Tipo | Observação |
|-------|------|------------|
| protocol | uint32 | versão do protocolo (1) |
| genesis | hex 32B | ID da rede — mismatch → reject + drop |
| height | uint64 | altura da ponta |
| cum_work | hex | trabalho acumulado (critério de quem sincroniza de quem) |
| listen_addr | string | endereço anunciável do peer |
| nonce | uint64 | detecção de auto-conexão |

## Regras de negócio

- Nenhuma mensagem processada antes do handshake completo (`version`+`verack`)
- Frame > 1 MiB → desconectar (proteção de memória)
- Bloco recebido → `chain.AcceptBlock`; órfão dispara `getheaders` para o peer
  de origem; inválido → `reject` (e futuro ban score — non-goal por ora)
- Tx recebida → `mempool.Add`; se aceita, re-anunciar via `inv` aos demais
  peers (menos o de origem)
- Novo bloco minerado/conectado localmente → `inv(block)` a todos os peers

## Interface do pacote

```go
func NewServer(cfg Config, c *chain.Chain, m *mempool.Mempool, p params.Params) *Server
func (s *Server) Start() error      // listener + dial seeds + sync loop
func (s *Server) Stop() error
func (s *Server) BroadcastTx(tx *core.Tx)
func (s *Server) BroadcastBlock(id [32]byte)
func (s *Server) PeerCount() int
```

## Casos de erro / edge cases

- Genesis/protocol mismatch → `reject` + drop imediato
- Auto-conexão (nonce igual) → drop silencioso
- Peer que não responde ping 2× → drop; peer manager redisca do address book
- Headers que não encadeiam ou com bits errados → drop do peer
- Dois peers anunciando o mesmo bloco → um único `getdata` (dedup por ID)

## Critérios de aceite

- [ ] `message.go`, `codec.go`, `peer.go`, `server.go`, `sync.go` + testes
- [ ] Teste unitário: handshake sobre `net.Pipe` (happy path, genesis
      mismatch, version mismatch)
- [ ] Teste: frame acima de 1 MiB rejeitado sem alocar
- [ ] Integração: 2 nodes in-process — A com 50 blocos, B vazio → B sincroniza
      até a ponta de A
- [ ] Integração: 2 nodes mineram forks separados, conectam, ambos convergem
      para a cadeia de mais trabalho
- [ ] Integração: tx submetida em A aparece no mempool de B
- [ ] `go test -race` verde

## Fora de escopo / não fazer

- Sem libp2p, sem descoberta via DNS seeds, sem NAT traversal
- Sem ban score/misbehavior nesta versão (evolução futura)
- Sem compact blocks — `inv`/`getdata` simples é suficiente no volume de dev

# Spec: p2p — protocolo de rede, peers e sincronização

> Domínio do node Zhu (ver [PLAN.md](../../PLAN.md)). Depende de `core`,
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
  então basta conhecer um nó para descobrir a rede. O **address book** guarda
  esses endereços com um mínimo de memória de qualidade (falhas consecutivas,
  última conexão boa) — é o "addrman" do Bitcoin em miniatura: endereço que
  falha espera cada vez mais para ser rediscado (backoff exponencial) e, se
  nunca responde, é esquecido. Novidades se espalham por **relay**: um endereço
  aprendido pela primeira vez (no handshake ou num `addr`) é re-anunciado aos
  demais vizinhos na hora — um node recém-chegado é conhecido pela rede inteira
  sem que ninguém reconecte. Assim a rede se **auto-recupera**: se o seed
  configurado cair, o node continua discando os endereços que aprendeu.

## Decisões & porquês (regra e arquitetura)

Este pacote é a superfície da rede — a única parte do node que fala com o mundo
não-confiável. As decisões equilibram simplicidade, proteção contra abuso e o
requisito de que o node doméstico atrás de NAT seja cidadão pleno.

- **TCP puro + stdlib, sem libp2p.** libp2p traria multiplexação, criptografia e
  descoberta prontas — e um universo de dependências, conceitos e superfície de
  bugs. Para o volume e o propósito didático do projeto, um framing próprio (4
  bytes de tamanho + JSON) sobre `net.Conn` é auditável de ponta a ponta e cabe
  na cabeça de quem está aprendendo. Cada mensagem é legível; não há mágica.
- **Envelope JSON com blocos/txs em base64, não um binário próprio de fio.** O
  consenso já tem serialização canônica em bytes (em `core`); o *transporte* não
  precisa ser compacto, precisa ser inspecionável. JSON deixa depurar a rede com
  um `tcpdump` legível; os bytes que importam (bloco/tx) viajam canônicos dentro
  dele. Um tipo de mensagem desconhecido é ignorado — binários antigos seguem
  compatíveis quando novos tipos surgem.
- **Frame limitado a 1 MiB, rejeitado sem alocar.** O tamanho vem nos primeiros 4
  bytes; um peer malicioso poderia anunciar 4 GiB e nos fazer alocar até
  estourar. Validar o tamanho *antes* de alocar o buffer fecha esse DoS. 1 MiB é
  folgado para qualquer mensagem legítima (bloco máx = 256 KiB).
- **Nada processado antes do handshake completo.** `version`+`verack` primeiro, e
  o `version` carrega o hash do gênesis. Rejeitar cedo quem está em outra rede (ou
  fala outro protocolo) evita gastar CPU/estado com uma conexão que nunca seria
  útil — e é onde a separação de redes por gênesis é aplicada.
- **Fork choice do sync espelha o da chain: sincroniza por `cum_work`, não por
  altura.** O node só entra em IBD se o trabalho acumulado do peer supera o nosso.
  Usar altura deixaria um peer com uma cadeia longa e fraca nos arrastar para o
  ramo errado; trabalho é o mesmo critério que a chain usa para decidir a ponta,
  então rede e disco concordam.
- **Headers-first com locator exponencial.** Baixar 96 bytes de header antes dos
  corpos deixa validar encadeamento e bits barato, e o *locator* (hashes
  espaçados exponencialmente a partir da ponta) acha o ponto em comum com o peer
  em O(log n) mensagens em vez de mandar a cadeia inteira. Argon2 (caro) só roda
  no bloco completo — coerente com a decisão registrada em `pow`.
- **Outbound-only é cidadão pleno (máx 8 outbound).** O design *não pode* assumir
  que peers são alcançáveis de fora, senão excluiria todo mundo atrás de
  NAT/CGCNAT — a maioria dos nodes domésticos, o público-alvo do projeto. Um node
  que só disca para fora valida, minera e propaga por essas conexões; aceitar
  entrada (port forward) é opcional e só aumenta a capilaridade. Por isso o
  address book só anuncia `listen_addr` de quem *declarou* aceitar entrada — não
  poluímos a rede com endereços que ninguém alcança.
- **Proxy SOCKS5 + `Advertise` separado para Tor.** Toda saída pode passar por um
  SOCKS5 (o Tor local, que resolve `.onion`), e o endereço anunciado no handshake
  é desacoplado do endereço de bind. Assim um hidden service divulga seu `.onion`
  em vez do `127.0.0.1` local, e o transporte sai cifrado de ponta a ponta pelo
  próprio onion — por isso não implementamos criptografia de transporte própria
  nesta fase.
- **Cap de conexões, ping/pong com drop, dedup de `getdata`, sem ban score
  (ainda).** Limites duros (8 outbound, 32 endereços por `addr`, drop após 2 pings
  sem resposta, um único `getdata` por bloco anunciado por vários peers) protegem
  memória e banda do node caseiro por construção. Ban score/misbehavior é
  evolução futura declarada — o mínimo viável fecha os abusos óbvios sem a
  complexidade de um sistema de reputação.
- **Address book com backoff exponencial e evicção — sem buckets tried/new.**
  Cada endereço carrega `Fails`/`NextTry`/`LastSeen` (só em runtime; persiste-se
  apenas a lista de endereços). Falha de dial dobra a espera (15s → 30s → ... →
  1h); na 10ª falha seguida o endereço é evicto — exceto **seeds** (`-peers`),
  que são a âncora do dono do node: entram em backoff mas nunca saem. Com o
  book cheio (256), um endereço novo evicta a pior entrada não-seed (mais
  falhas; empate → visto há mais tempo) em vez de ser descartado — senão lixo
  anunciado por um peer malicioso ocuparia as vagas para sempre. Reaprender um
  endereço já conhecido **não** reseta o backoff dele (re-anunciar um morto não
  o ressuscita). É a essência do addrman do Bitcoin sem os buckets
  tried/new, feelers e aleatorização — proteção anti-eclipse completa fica como
  evolução futura, junto do ban score.
- **Addr relay + `getaddr` periódico — descoberta contínua, não só no
  handshake.** Sem isso, o `getaddr` único do handshake congela a visão da
  rede: quem conectou primeiro nunca ficaria sabendo de quem chegou depois, e a
  morte do seed isolaria os antigos. Só endereços **novos no book** são
  relayados (o eco volta, já é conhecido, morre ali — o gossip converge em vez
  de circular), nunca o próprio `advertise` nem o endereço do destinatário.
  Como redundância barata para relays perdidos (node estava offline, peer caiu
  no meio), um `getaddr` é enviado a UM peer sorteado a cada `AddrInterval`
  (10 min) — uma mensagem de até 32 endereços; nada que pese no node caseiro.

## Objetivo

Conectar nós entre si, propagar blocos e transações, e sincronizar a cadeia de
qualquer nó novo até a ponta — sem coordenador central.

## Escopo

Entra:
- `message.go` — envelope `{Type string, Payload json.RawMessage}`; tipos:
  `version`, `verack`, `ping`, `pong`, `getaddr`, `addr`, `inv`, `getdata`,
  `getheaders`, `headers`, `block`, `tx`, `reject`, `getmempool`. Blocos/txs
  viajam como bytes canônicos base64 dentro do JSON
- `codec.go` — frame: 4 bytes big-endian de tamanho + JSON; frame máx 1 MiB;
  funciona sobre `io.ReadWriter` (testável com `net.Pipe`)
- `peer.go` — loop por peer, estado do handshake, ping/pong a cada 2 min
  (drop após 2 falhas), detecção de auto-conexão por nonce
- `server.go` — listener TCP, peer manager (máx 8 outbound; seeds do
  `--peers` com prioridade), address book persistido no bucket `meta` da
  chain com backoff exponencial por endereço e evicção de mortos (seeds nunca
  saem), gossip de `addr` (até 32 endereços; endereços novos são relayados aos
  vizinhos e um `getaddr` de refresh sai a cada 10 min); toda saída pode passar por um proxy
  SOCKS5 (`Config.Proxy` — o Tor, com o proxy resolvendo `.onion`), e
  `Config.Advertise` troca o endereço anunciado no handshake (um hidden
  service divulga o `.onion`, não o `127.0.0.1` local)
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
- Criptografia de transporte (rede de dev; anotar como evolução futura —
  via Tor o transporte já sai cifrado de ponta a ponta pelo próprio onion)

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
- `getmempool` (1× por conexão): enviado no handshake se já estamos em dia
  com o peer, ou ao fim do IBD (antes disso as pendentes não validariam);
  resposta = `inv` com até 1.000 txids (maior fee rate primeiro) — o fluxo
  `getdata`→`tx` normal completa. Tipo desconhecido é ignorado, então
  binários antigos seguem compatíveis

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
  (respeitando o backoff do endereço)
- Endereço que falha dial/handshake → backoff exponencial (15s → ... → 1h);
  10 falhas seguidas → evicto do book (seed nunca — só espera o backoff)
- Address book cheio (256) + endereço novo → evicta a pior entrada não-seed
- `addr` re-anunciando endereço já conhecido → ignorado (não reseta o backoff
  nem é re-relayado — é o que faz o gossip de endereços convergir)
- Headers que não encadeiam ou com bits errados → drop do peer
- Dois peers anunciando o mesmo bloco → um único `getdata` (dedup por ID)

## Critérios de aceite

- [x] `message.go`, `codec.go`, `peer.go`, `server.go`, `sync.go` + testes
- [x] Teste unitário: handshake sobre `net.Pipe` (happy path, genesis
      mismatch, version mismatch, auto-conexão por nonce)
- [x] Teste: frame acima de 1 MiB rejeitado sem alocar
- [x] Integração: 2 nodes in-process — A com 50 blocos, B vazio → B sincroniza
      até a ponta de A
- [x] Integração: 2 nodes mineram forks separados, conectam, ambos convergem
      para a cadeia de mais trabalho
- [x] Integração: tx submetida em A aparece no mempool de B (+ bloco novo
      propaga via inv/getdata)
- [x] Teste: falha de dial incrementa `Fails` e empurra `NextTry` (backoff);
      sucesso zera; morto não-seed é evicto e seed permanece; book cheio
      evicta a pior entrada ao aprender endereço novo
- [x] Integração: node que entra depois é aprendido via relay por quem já
      estava conectado; seed morre e os demais se conectam entre si; endereço
      já conhecido não é re-relayado (eco morre)
- [x] `go test -race` verde

## Fora de escopo / não fazer

- Sem libp2p, sem descoberta via DNS seeds, sem NAT traversal
- Sem ban score/misbehavior nesta versão (evolução futura)
- Sem compact blocks — `inv`/`getdata` simples é suficiente no volume de dev

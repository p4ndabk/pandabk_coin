# Tutorial — conectando um Mac pelo 4G ao seu node, via Tor

> Cenário deste guia: **você** roda um node Zhu na sua casa e um **amigo
> com um Mac na rede 4G** vai conectar nele, minerar junto e servir de
> segundo peer — se a sua internet cair, o node dele continua e a rede
> segue viva. Tudo **via Tor**: ninguém expõe IP, ninguém abre porta em
> roteador, ninguém depende de serviço de terceiros. Primeira vez com o
> node? Comece pelo [TUTORIAL.md](TUTORIAL.md); a referência completa de
> Tor é a [seção 9 da documentação](docs/README.md#9-usando-com-tor).

## Por que Tor resolve o problema

Rede 4G usa **CGNAT**: o Mac do seu amigo não tem IP público, é impossível
"abrir porta" do lado dele — e do seu lado, mexer em roteador dá trabalho e
também falha com CGNAT. O Tor contorna os dois lados de uma vez:

- **Você** vira um *hidden service*: ganha um endereço `xyz...onion:9551`
  que funciona de qualquer lugar do mundo, sem abrir porta. Só a porta do
  Zhu fica alcançável — e quem conecta nem sabe seu IP.
- **O amigo** disca esse `.onion` com o flag `-proxy` do node (suporte
  nativo a SOCKS5 — funciona no macOS sem gambiarras), e o IP dele também
  não aparece para ninguém.

O resto o node já faz sozinho: quando a conexão cai ele **redisca
automaticamente**, e quando os dois lados se reencontram o sync
**reconcilia** — vence a chain com mais trabalho acumulado.

## Passo 0 — Você: prepare o pacote para o Mac dele

Na raiz do projeto:

```sh
scripts/build-macos.sh
```

Sai o `dist/zhu-<versão>-macos.tar.gz` com binários para Apple Silicon e
Intel, `zhu.conf`, instalador (que já remove a quarentena do Gatekeeper),
LEIA-ME e SHA256SUMS. Envie esse `.tar.gz` para o seu amigo.

> **Importante:** os dois lados precisam rodar o **mesmo binário (mesma
> versão)** e o **mesmo perfil** (`devnet` neste teste) — o handshake
> rejeita genesis/perfil diferentes.

## Passo 1 — Você: vire um hidden service

Instale e suba o Tor (uma vez só):

```sh
brew install tor
```

Edite o `torrc` (`/opt/homebrew/etc/tor/torrc` em Apple Silicon,
`/usr/local/etc/tor/torrc` em Mac Intel) e acrescente:

```
HiddenServiceDir /opt/homebrew/var/lib/tor/zhu/
HiddenServicePort 9551 127.0.0.1:9551
```

Suba o Tor e leia seu endereço onion:

```sh
brew services start tor
cat /opt/homebrew/var/lib/tor/zhu/hostname
# exemplo: zhuxyzabc...def.onion
```

Agora rode o node escutando **só localmente** (o mundo externo não vê a
porta — só o onion chega nela, via Tor) e anunciando o onion aos peers:

```sh
bin/zhu run -profile devnet \
  -listen 127.0.0.1:9551 \
  -advertise zhuxyzabc...def.onion:9551
```

Passe o endereço `zhuxyzabc...def.onion:9551` para o seu amigo — esse é
o seu node na rede, e ele não revela nem seu IP nem sua cidade.

## Passo 2 — O amigo: instale Tor + node e conecte

No Mac dele, primeiro o Tor (só precisa do daemon rodando; nada de
configurar hidden service do lado dele):

```sh
brew install tor && brew services start tor
```

Depois o node:

```sh
tar xzf zhu-<versão>-macos.tar.gz
cd zhu-<versão>-macos
./instalar.sh
```

Edite o `zhu.conf` apontando para o seu onion, com a saída roteada pelo
Tor local:

```
profile=devnet
proxy=127.0.0.1:9050
peers=zhuxyzabc...def.onion:9551
listen=
```

(`listen=` vazio: o node dele só faz conexões de saída — atrás do CGNAT do
4G é exatamente o que funciona, e continua um cidadão pleno da rede:
valida, minera e propaga.)

E suba:

```sh
zhu run
```

Prefere tela em vez de terminal? O **Zhu Desktop** tem os mesmos campos
na primeira abertura e na aba Ajustes — "Conectar a (peers)" recebe o
`.onion` e "Proxy SOCKS5 — Tor" recebe `127.0.0.1:9050`.

O que acontece no primeiro `run`:

- A **wallet dele nasce automaticamente** no datadir (`~/.zhu`) — anote
  as 12 palavras que aparecem uma única vez.
- O node atravessa o Tor, conecta no seu onion, **sincroniza a chain** e
  começa a **minerar** (ligado por padrão) — a coinbase paga a wallet dele.

## Passo 3 — Confirmem que a rede existe

Nos logs dos dois lados deve aparecer o aperto de mão:

```
🤝 peer … conectado (…) — altura declarada N
```

E em outro terminal, de cada lado:

```sh
zhu info
```

A **altura** dos dois deve convergir para o mesmo número. Via Tor o
primeiro circuito leva alguns segundos e o sync inicial é mais lento que
numa conexão direta — é o preço do anonimato, e para um node doméstico não
faz diferença no dia a dia.

## O teste que você quer fazer: derrubar a sua internet

1. Desligue sua internet (ou pare seu node). No Mac do amigo o peer some do
   log, mas **o node dele segue minerando sozinho** — a chain dele continua
   crescendo.
2. Religue. Você não precisa fazer nada: o node dele redisca o `.onion`
   automaticamente a cada intervalo, o Tor refaz o circuito e os dois
   **reconciliam** — a chain com mais trabalho acumulado vence, o outro
   lado reorganiza.
3. É normal que blocos minerados pelo lado "perdedor" durante a separação
   virem **órfãos** (e a recompensa deles suma). Numa rede de 2 nodes de
   teste isso é esperado e é exatamente o comportamento que você quer
   observar.

## Dicas para o lado 4G

- **Consumo de dados é pequeno**: bloco tem no máximo 256 KiB e no devnet o
  ritmo é baixo; o overhead do Tor não muda a ordem de grandeza. A
  mineração em si não usa rede, só CPU.
- **Não deixe o Mac dormir**, senão o node (e a mineração) param. Para um
  teste longo:

  ```sh
  caffeinate -is zhu run
  ```

- IP do 4G muda o tempo todo — **irrelevante aqui**: o `.onion` é estável e
  o lado dele nem tem endereço para mudar (só faz saída).
- Se o 4G oscilar, sem pânico: a reconexão e o re-sync são automáticos,
  mesmo mecanismo do teste de queda acima.

## Problemas comuns

| Sintoma | Causa provável | Conserto |
|---|---|---|
| `handshake … falhou` no log | Perfil ou versão diferente entre os lados | Mesmos binário e `profile=devnet` nos dois |
| Nenhum peer, nada no log | Daemon `tor` parado em um dos lados | `brew services start tor` (proxy 9050 do lado dele; hidden service do seu) |
| Onion não responde | Hidden service mal configurado | Conferir `torrc`, reiniciar o Tor, `cat …/hostname` de novo |
| Conecta e cai logo | Circuito Tor instável no 4G | Normal em rede móvel — o redial resolve sozinho |
| Altura não converge | Um dos lados parado ou sem peer | `zhu info` nos dois e olhar o log do 🤝 |

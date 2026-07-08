# cmd/desktop — SPEC

## Conceito

O `panda-desktop` é a **janela** do node: tudo que a CLI faz no terminal
(`info`, `balance`, `send`), numa interface gráfica nativa — para o amigo
que nunca abriu um terminal ter um node em casa com dois cliques.

Duas ideias sustentam o design:

1. **Híbrido**: ao abrir, o app procura um node já em execução na RPC local
   (`getinfo` com timeout curto). Achou → vira um **painel** do node externo.
   Não achou → **embute o node no próprio processo** (importa
   `internal/node`; é tudo Go) com a mesma config do CLI (flags > panda.conf
   > env `NODE_*` > defaults). Fechar a janela desliga o node embutido na
   ordem segura.
2. **Um caminho só de dados**: nos dois modos a GUI fala com o node pela
   RPC JSON local (`internal/rpcclient`) — o mesmo canal da CLI. O bbolt é
   single-writer; ninguém abre o banco de um node vivo.

A interface segue o pedido do dono do projeto: **extremamente clean,
inspiração Apple** — tema claro/escuro com um acento só (azul), fonte Inter
embutida, cartões com números grandes em vez de tabelas de servidor. O app
deve parecer uma carteira, não um terminal.

## Decisões & porquês (regra e arquitetura)

O desktop existe para uma pessoa que nunca abriu um terminal ter um node em
casa. Cada decisão remove uma fricção sem duplicar o que o node já faz.

- **Um caminho só de dados: a GUI fala com o node pela mesma RPC local da CLI.**
  Nos dois modos (painel de node externo ou node embutido) a interface usa
  `internal/rpcclient` — o mesmo canal do `panda-node info/balance/send`. Isso
  evita uma segunda implementação de acesso ao estado (e uma segunda fonte de
  bugs), e respeita o bbolt single-writer: a GUI nunca abre o banco de um node
  vivo. A regra "ninguém além do processo do node escreve no banco" é a mesma do
  CLI, herdada de graça.
- **Híbrido: detecta node externo, senão embute o node no próprio processo.** Ao
  abrir, tenta `getinfo` com timeout curto. Achou → vira painel do node que já
  roda (não sobe um segundo node competindo pelo datadir). Não achou → importa
  `internal/node` e roda o node dentro da própria janela. É tudo Go, então
  embutir é só uma chamada — o usuário não precisa saber que "node" e "app" são
  coisas distintas. Fechar a janela desliga o node embutido na ordem segura
  (p2p→miner→RPC→bbolt), a mesma do `run`.
- **Config idêntica à do CLI (flag > panda.conf > env > default), ancorada em
  `~/.panda/panda.conf`.** Reusar a precedência do node significa que a GUI e o
  terminal enxergam o mesmo node com a mesma config — editar pela aba Ajustes é
  editar o mesmo arquivo que o CLI lê. Ancorar num caminho fixo (não no diretório
  de abertura) é o que faz o clique duplo no Finder funcionar; `-config` e um
  panda.conf no diretório atual ainda vencem, para não quebrar o fluxo CLI.
- **Regras de consenso do build.conf valem também no desktop.** O
  `scripts/build-desktop.sh` reusa os mesmos `-ldflags` do node. Se o desktop
  pudesse ser compilado com regras diferentes do node, o app formaria uma rede
  separada do binário CLI do mesmo pacote — exatamente o acidente que o gênesis
  derivado existe para impedir. Um build, uma rede.
- **Tela de pré-configuração antes do primeiro boot.** Sem panda.conf salvo, o
  app pede peers/mineração/portas *antes* de ligar o node, em vez de subir com
  defaults e deixar o usuário leigo sem peers (um node sozinho não faz nada
  visível). A primeira experiência é "configurei e conectei", não "abri e nada
  aconteceu".
- **Não-objetivos v1 declarados (liga/desliga mineração, tray, gráficos,
  cross-compile da GUI).** Ligar mineração pela GUI pede uma RPC `setmining` que
  ainda não existe (v2); tray/gráficos/i18n são polimento; a GUI usa cgo e por
  isso é buildada nativa em cada OS (`fyne-cross` opcional). Cada corte mantém a
  v1 focada em "ver o node e enviar PANDA", que é o que prova o produto.

## Escopo (v1)

- [x] Decisão híbrida: painel de node externo OU node embutido
- [x] Aba **Início**: cartões (saldo, altura, dificuldade, peers, hashrate),
      recompensa/halving/alvo, endereço, indicador do modo — refresh 2s
- [x] Aba **Carteira**: endereço + copiar, saldo/gastável, aviso de backup
      com o caminho do wallet.json
- [x] Aba **Enviar**: destino/valor/taxa → confirmação → txid ou erro
- [x] Aba **Atividade**: logs do node embutido ao vivo (peers, blocos,
      retarget, halving); em modo painel, aponta para o terminal do node
- [x] Tema Apple-clean (claro/escuro automático) com Inter (SIL OFL,
      licença embutida em assets/)
- [x] **Primeira vez**: sem panda.conf salvo → tela de pré-configuração
      (peers, mineração, portas, datadir) ANTES de ligar o node; salvar cria
      o arquivo e inicia
- [x] Aba **Ajustes**: edita o panda.conf pela interface (mesmas chaves da
      CLI) com "salvar e reiniciar o node embutido"
- [x] panda.conf do desktop ancorado em `~/.panda/panda.conf` (clique duplo
      funciona); `-config` e um panda.conf no diretório atual ainda vencem
- [x] Fechar janela → shutdown limpo do node embutido (p2p→miner→RPC→bbolt)
- [x] Regras de consenso do build.conf valem também aqui
      (scripts/build-desktop.sh reusa os mesmos -ldflags)

## Não-objetivos (v1)

- Ligar/desligar mineração pela GUI (pede RPC `setmining` — v2)
- Bandeja/tray, gráficos, auto-update, i18n, empacotamento .app/.msi
- Cross-compile da GUI (cgo): buildar nativo em cada OS; `fyne-cross` é
  opcional e documentado

## Critérios de aceite

- `go build ./cmd/desktop` (cgo) compila; `go vet`/`go test ./...` verdes;
  `CGO_ENABLED=0 go build ./cmd/node` continua estático.
- Sem node no ar: abrir o app sobe node embutido e o Início atualiza.
- Com `node run` ativo: o app conecta como painel sem tocar no datadir.
- Enviar PANDA pela aba Enviar aparece no `balance` do destinatário.
- Fechar a janela com node embutido: chain reabre limpa na próxima vez.

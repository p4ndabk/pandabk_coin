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

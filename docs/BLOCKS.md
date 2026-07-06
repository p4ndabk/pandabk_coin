# O banco de dados do node — `chain.db` por dentro

O banco do node é o arquivo único `~/.panda/chain.db`, no formato **bbolt**
— um banco chave→valor embutido, escrito em Go puro. Não existe servidor
SQL rodando ao lado: o banco é uma biblioteca dentro do próprio node, e o
arquivo é tudo. Pensa nele como um armário com **6 gavetas** (buckets),
cada uma com um papel.

## As 6 gavetas

### 1. `blocks` — o arquivo morto (a verdade bruta)

```
hash do bloco (32 bytes)  →  o bloco inteiro, serializado
```

Cada bloco que já passou na validação vai aqui, **com as transações
dentro** — não existe tabela de transações separada; o bloco É o registro.
Inclui blocos de ramos perdedores (laterais): histórico não se apaga, só
se desativa.

### 2. `blockIndex` — a ficha catalográfica

```
hash  →  altura (8B) + status (1B) + trabalho acumulado
```

O status é 1 byte: **1 = ativo** (faz parte da corrente principal), **2 =
lateral** (ramo perdedor, guardado caso um dia vire vencedor), **3 =
inválido** (marcado para nunca mais perder tempo com ele). O trabalho
acumulado é a soma de "dificuldade gasta" do gênesis até ali — **é ele que
decide qual corrente vence** num fork.

### 3. `heightIndex` — a régua da corrente ativa

```
altura (8 bytes)  →  hash
```

Responde "qual é o bloco 42?" instantaneamente — mas **só para a corrente
ativa**. É a gaveta que o `node block 42`, o `getrecentblocks` e o
retarget consultam. Num reorg, é ela que é reescrita: as alturas passam a
apontar para os hashes do ramo novo.

### 4. `utxo` — o cofre (onde "estão" as moedas)

```
txid + índice do output (36 bytes)  →  valor (8B) + dono/PKH (20B) + altura (8B) + é-coinbase (1B)
```

Cada moeda não gasta = uma entrada de **37 bytes**. Seu saldo não está
escrito em lugar nenhum — o `getbalance` **varre esta gaveta** somando as
entradas cujo dono é o hash da sua chave pública. Gastar = apagar entradas
daqui e criar novas para o destinatário. É o estado vivo da economia
inteira da rede.

### 5. `undo` — o desfazer (o segredo do reorg)

```
hash do bloco  →  lista dos UTXOs que ele APAGOU
```

Quando um bloco conecta, os UTXOs que ele gasta somem da gaveta 4 — mas
ficam anotados aqui. Se um dia esse bloco precisar ser *desconectado* (o
ramo dele perdeu a corrida de trabalho), o undo devolve cada moeda apagada
exatamente como era. Sem esta gaveta, reorg seria impossível sem revalidar
a chain inteira do zero.

### 6. `meta` — post-its

```
"tip"      →  hash da ponta atual
"addrbook" →  JSON com os peers conhecidos
```

Miudezas de estado: onde a corrente termina agora, e a agenda de endereços
que a fofoca `getaddr` foi enchendo (é por ela que o node reencontra a
rede sozinho depois de reiniciar).

## Como um bloco atravessa o armário

Bloco chega (minerado aqui ou vindo da rede) → validação completa → **UMA
transação atômica do bbolt** faz tudo de uma vez:

1. grava o bloco em `blocks`;
2. cria a ficha em `blockIndex`;
3. aponta a altura em `heightIndex`;
4. apaga os UTXOs gastos e cria os novos em `utxo`;
5. anota o desfazer em `undo`;
6. move o `tip` em `meta`.

**Ou tudo entra, ou nada entra.** Puxou o cabo da tomada no meio, o
arquivo reabre íntegro no estado anterior. O reorg inteiro — desconectar N
blocos do ramo perdedor, reconectar M do vencedor — também é uma única
transação.

## Por que um escritor por vez

O bbolt tranca o arquivo para **um único processo**. Consequências
práticas:

- `node balance`/`info`/desktop **nunca abrem o chain.db** — perguntam ao
  node vivo pela RPC local;
- dois nodes no mesmo datadir = erro imediato ("outro node rodando no
  mesmo datadir?");
- nunca mova/renomeie o `chain.db` com o node aberto — o processo continua
  escrevendo no arquivo antigo pelo file handle.

## Fuçando com as próprias mãos

Com o node **parado**:

```sh
go run go.etcd.io/bbolt/cmd/bbolt buckets ~/.panda/chain.db   # lista as gavetas
go run go.etcd.io/bbolt/cmd/bbolt stats  ~/.panda/chain.db    # tamanhos e páginas
```

Mas a janela amigável é a que o próprio node oferece: a aba **Blocos** do
desktop e o `node block` são um leitor das gavetas 1+3, e o
`getbalance` é um somador da gaveta 4.

## Contexto: como o Bitcoin faz

O Bitcoin Core usa LevelDB com a mesma arquitetura conceitual — blocos
brutos + índice + UTXO set + undo files. Chegamos no mesmo desenho pela
mesma necessidade, só que num arquivo único e com o código-fonte legível
em uma sentada: `internal/chain/store.go`.

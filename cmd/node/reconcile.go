// reconcile é o "fork choice" simplificado da demo de rede (powdemo -peer):
// sem autoridade única, cada máquina mina e aceita seus próprios blocos
// localmente. Quando duas chains divergem (uma partição de rede, ou as duas
// mineraram ao mesmo tempo sem se ver), reconcile decide quem vence
// comparando TRABALHO ACUMULADO a partir do ponto de divergência — a mesma
// regra "heaviest chain wins" de qualquer rede PoW — e o lado mais fraco
// descarta seu próprio trecho e adota o do peer. É uma pré-visualização
// honesta, porém simplificada, do fork choice de verdade que chega no M2
// (que soma trabalho sobre uma chain validada em bbolt, não duas tabelas
// SQLite comparadas por RPC).
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"pandabk_coin/internal/pow"
)

// reconcile compara a chain local com a do peer e adota a mais pesada a
// partir do ponto de divergência. Erro de rede (peer inalcançável) sobe pro
// chamador decidir — não corrompe nada localmente.
func reconcile(local *demoStore, peer *netStore, name string) error {
	localTip, err := local.tip()
	if err != nil {
		return fmt.Errorf("tip local: %w", err)
	}
	peerTip, err := peer.tip()
	if err != nil {
		return err
	}
	if localTip.height == peerTip.height && localTip.id == peerTip.id {
		return nil // já em sincronia
	}

	// Acha o ponto de divergência andando pra trás a partir da menor altura
	// em comum, comparando o ID do bloco em cada altura.
	h := localTip.height
	if peerTip.height < h {
		h = peerTip.height
	}
	for h > 0 {
		lb, lerr := local.blockAt(h)
		pb, perr := peer.blockAt(h)
		if lerr == nil && perr == nil && lb.id == pb.id {
			break
		}
		h--
	}
	forkHeight := h

	localWork, err := chainWork(local, forkHeight+1, localTip.height)
	if err != nil {
		return fmt.Errorf("somando trabalho local: %w", err)
	}
	peerWork, err := chainWork(peer, forkHeight+1, peerTip.height)
	if err != nil {
		return fmt.Errorf("somando trabalho do peer: %w", err)
	}

	adoptPeer := false
	switch localWork.Cmp(peerWork) {
	case -1:
		adoptPeer = true
	case 0:
		// Empate exato de trabalho (ex: os dois lados vazios, ou uma
		// coincidência rara): desempata com uma regra que os dois lados
		// concordam sem precisar conversar de novo — senão nenhum cede e a
		// divergência nunca se resolve.
		adoptPeer = localTip.height > 0 && peerTip.height > 0 &&
			hex.EncodeToString(peerTip.id[:]) < hex.EncodeToString(localTip.id[:])
	}
	if !adoptPeer {
		return nil // sua chain já vence (ou empatou a seu favor) — nada a fazer
	}

	discarded := localTip.height - forkHeight
	if err := local.truncateAbove(forkHeight); err != nil {
		return fmt.Errorf("descartando %d bloco(s) local(is): %w", discarded, err)
	}
	adopted := uint64(0)
	for hh := forkHeight + 1; hh <= peerTip.height; hh++ {
		row, err := peer.blockAt(hh)
		if err != nil {
			return fmt.Errorf("buscando bloco %d do peer: %w", hh, err)
		}
		if err := local.insertBlock(row); err != nil {
			return fmt.Errorf("gravando bloco %d: %w", hh, err)
		}
		adopted++
	}
	if discarded > 0 {
		fmt.Printf("🔀 [%s] %s: reorg no bloco %d — o peer tinha mais trabalho acumulado; "+
			"adotando %d bloco(s) dele, descartando %d seu(s)\n\n",
			time.Now().Format("15:04:05"), name, forkHeight+1, adopted, discarded)
	} else {
		fmt.Printf("📥 [%s] %s: adotando %d bloco(s) novo(s) do peer\n\n",
			time.Now().Format("15:04:05"), name, adopted)
	}
	return nil
}

// chainWork soma o trabalho (pow.BlockWork, ~2^256/(target+1)) de cada bloco
// no intervalo [from, to] — o "peso" real de um trecho da chain, a mesma
// métrica que decide qual fork vence numa rede PoW de verdade. from > to
// (intervalo vazio) soma zero.
func chainWork(s raceStore, from, to uint64) (*big.Int, error) {
	total := new(big.Int)
	for h := from; h <= to; h++ {
		b, err := s.blockAt(h)
		if err != nil {
			return nil, err
		}
		total.Add(total, pow.BlockWork(b.bits))
	}
	return total, nil
}

// peerLoop mantém o node reconciliado com o peer indefinidamente: tenta a
// cada segundo, e só fala quando o estado muda (peer caiu / peer voltou) —
// fica quieto durante uma queda longa, em vez de martelar o log.
func peerLoop(ctx context.Context, local *demoStore, peer *netStore, name string, startedUp bool) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	up := startedUp
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := reconcile(local, peer, name)
			switch {
			case err == nil && !up:
				fmt.Printf("🔌 [%s] %s: peer voltou — sincronizado de novo\n\n", time.Now().Format("15:04:05"), name)
				up = true
			case err != nil && up:
				fmt.Printf("📴 [%s] %s: peer inalcançável — minerando de forma independente até ele voltar\n\n",
					time.Now().Format("15:04:05"), name)
				up = false
			}
		}
	}
}

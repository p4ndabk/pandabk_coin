package node

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"

	"pandabk_coin/internal/core"
)

// O mempool vive só em RAM — sem estes dois passos, desligar o node jogaria
// fora toda transação ainda não confirmada (o mesmo papel do mempool.dat do
// Bitcoin Core). saveMempool roda no Stop, loadMempool no Start; cada tx
// recarregada passa pela validação COMPLETA do mempool de novo, porque a
// chain pode ter andado enquanto o node dormia (a tx pode já ter confirmado,
// ou virado inválida).

func (n *Node) saveMempool() {
	txs := n.mp.AllTxs()
	path := n.cfg.MempoolPath()
	if len(txs) == 0 {
		_ = os.Remove(path)
		return
	}
	raws := make([]string, len(txs))
	for i, tx := range txs {
		raws[i] = hex.EncodeToString(tx.Bytes())
	}
	data, err := json.Marshal(raws)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("⚠️  não consegui salvar o mempool (%d pendentes perdidas): %v", len(txs), err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("⚠️  não consegui salvar o mempool (%d pendentes perdidas): %v", len(txs), err)
		return
	}
	log.Printf("💾 %d transação(ões) pendente(s) salvas em %s", len(txs), path)
}

func (n *Node) loadMempool() {
	path := n.cfg.MempoolPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return // sem arquivo = nada pendente do último shutdown
	}
	_ = os.Remove(path) // consumido: o próximo Stop regrava o estado atual
	var raws []string
	if err := json.Unmarshal(data, &raws); err != nil {
		log.Printf("⚠️  %s corrompido — pendentes descartadas: %v", path, err)
		return
	}
	restored, dropped := 0, 0
	for _, r := range raws {
		raw, err := hex.DecodeString(r)
		if err != nil {
			dropped++
			continue
		}
		tx, err := core.DecodeTx(raw)
		if err != nil {
			dropped++
			continue
		}
		if err := n.mp.Add(&tx); err != nil {
			dropped++ // já confirmou, ou ficou inválida — descartar é o certo
			continue
		}
		restored++
	}
	if restored > 0 || dropped > 0 {
		log.Printf("📨 mempool restaurado: %d pendente(s) de volta na fila, %d descartada(s)", restored, dropped)
	}
}

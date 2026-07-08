// Package rpcclient é o cliente da RPC JSON local do node Zhu — o mesmo
// envelope {"method","params"} → {"result"}|{"error"} servido em /rpc.
// Usado pela CLI (cmd/node info/balance/send) e pelo app de desktop
// (cmd/desktop): ninguém abre o bbolt de um node em execução; conversa-se
// com o processo dono do banco.
package rpcclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Call chama method no node em addr (host:porta da RPC local) e decodifica
// o result em out (nil = descarta). Erros de rede viram uma mensagem clara
// de "o node está rodando?"; erros do node viram o texto enviado por ele.
func Call(addr, method string, params any, out any) error {
	return CallTimeout(addr, method, params, out, 10*time.Second)
}

// CallTimeout é o Call com timeout próprio — o app de desktop usa um curto
// para decidir depressa se já existe um node no ar.
func CallTimeout(addr, method string, params any, out any, timeout time.Duration) error {
	req := struct {
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post("http://"+addr+"/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("não consegui falar com o node em %s — ele está rodando? (%v)", addr, err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("resposta inválida do node: %w", err)
	}
	if envelope.Error != nil {
		return errors.New(envelope.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

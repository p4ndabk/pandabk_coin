package rpcclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeNode simula o /rpc do node: devolve result para "getinfo", error
// para "explode" e ecoa os params em "echo".
func fakeNode(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("request inválida: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "getinfo":
			_, _ = w.Write([]byte(`{"result":{"height":42}}`))
		case "echo":
			_, _ = w.Write([]byte(`{"result":` + string(req.Params) + `}`))
		default:
			_, _ = w.Write([]byte(`{"error":{"code":"error","message":"método desconhecido"}}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func addrOf(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestCallDecodesResult(t *testing.T) {
	srv := fakeNode(t)
	var out struct {
		Height uint64 `json:"height"`
	}
	if err := Call(addrOf(srv), "getinfo", nil, &out); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Height != 42 {
		t.Fatalf("height = %d, esperava 42", out.Height)
	}
}

func TestCallSendsParamsAndSurfacesNodeError(t *testing.T) {
	srv := fakeNode(t)
	var echoed map[string]string
	if err := Call(addrOf(srv), "echo", map[string]string{"to": "P123"}, &echoed); err != nil {
		t.Fatalf("Call echo: %v", err)
	}
	if echoed["to"] != "P123" {
		t.Fatalf("params não chegaram: %v", echoed)
	}
	err := Call(addrOf(srv), "inexistente", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "método desconhecido") {
		t.Fatalf("erro do node deveria ser repassado, veio %v", err)
	}
}

func TestCallNodeDownIsFriendly(t *testing.T) {
	err := CallTimeout("127.0.0.1:1", "getinfo", nil, nil, 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "ele está rodando?") {
		t.Fatalf("node fora do ar deveria dar mensagem amigável, veio %v", err)
	}
}

package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// runDocs acende a documentação web da Zhu: serve a pasta docs/ (a página
// única docs/index.html, com a identidade da rede) num servidor HTTP
// localhost-only e abre o navegador. É o mesmo espírito da RPC — interface
// local do dono, nada exposto pra fora. "Um comando no console" para ler a
// doc sem procurar o arquivo.
func runDocs(args []string) {
	fs := flag.NewFlagSet("docs", flag.ExitOnError)
	dir := fs.String("dir", "docs", "pasta da documentação (contém index.html)")
	addr := fs.String("addr", "127.0.0.1:8600", "endereço local para servir (loopback)")
	open := fs.Bool("open", true, "abrir o navegador automaticamente")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `uso: zhu docs [-dir docs] [-addr 127.0.0.1:8600] [-open=false]
  Sobe a documentação web da Zhu em localhost e abre o navegador.
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	root, err := findDocsDir(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Zhu não achou a documentação: %v\n", err)
		fmt.Fprintln(os.Stderr, "rode a partir da raiz do repositório, ou passe -dir CAMINHO.")
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Zhu não conseguiu abrir %s: %v\n", *addr, err)
		os.Exit(1)
	}
	url := "http://" + ln.Addr().String() + "/"

	fmt.Print(banner)
	fmt.Printf("\nDocumentação acesa em %s\n", url)
	fmt.Printf("servindo %s — Ctrl-C para apagar a luz.\n", root)

	if *open {
		openBrowser(url)
	}

	srv := &http.Server{Handler: http.FileServer(http.Dir(root))}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "servidor de docs parou: %v\n", err)
		os.Exit(1)
	}
}

// findDocsDir resolve a pasta de docs. Se o caminho preferido tem um
// index.html, usa. Senão sobe pelos diretórios pais (útil quando o binário
// roda de uma subpasta do repo) procurando um docs/index.html.
func findDocsDir(pref string) (string, error) {
	if hasIndex(pref) {
		return pref, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	name := filepath.Base(pref) // normalmente "docs"
	d := cwd
	for i := 0; i < 6; i++ {
		cand := filepath.Join(d, name)
		if hasIndex(cand) {
			return cand, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "", fmt.Errorf("%q não contém index.html (nem os diretórios acima)", pref)
}

func hasIndex(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}

// openBrowser tenta abrir a URL no navegador padrão do sistema. Best-effort:
// se falhar, o usuário ainda tem a URL impressa no terminal.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

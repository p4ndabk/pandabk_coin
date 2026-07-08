// Package wallet guarda a chave do usuário e constrói transações assinadas.
// Uma carteira não guarda moedas — guarda a CHAVE que destranca os UTXOs
// registrados na blockchain. Perder o arquivo = perder os fundos; por isso
// wallet.json nasce com permissão 0600 e New nunca sobrescreve um arquivo
// existente. Ver internal/wallet/SPEC.md.
package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"zhu/internal/core"
)

var (
	ErrWalletExists = errors.New("wallet: arquivo já existe — não sobrescrevo uma chave")
	ErrCorrupt      = errors.New("wallet: arquivo corrompido ou inconsistente")
	ErrBadPerm      = errors.New("wallet: permissão do arquivo insegura — esperado 0600")
)

type Wallet struct {
	priv     *ecdsa.PrivateKey
	pub      []byte // comprimida, 33 bytes
	pkh      [20]byte
	addr     string
	mnemonic string // 12 palavras, se a wallet nasceu/foi restaurada delas
}

// walletFile é o formato em disco (ver SPEC): chave privada PKCS8/base64,
// pública e endereço redundantes para inspeção e checagem de consistência.
// O mnemonic (se houver) fica no MESMO arquivo por decisão de conveniência:
// quem lê o arquivo já leva a chave de qualquer forma — mas fica legível
// por humanos, então o arquivo merece o mesmo sigilo de sempre.
type walletFile struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	Address    string `json:"address"`
	Mnemonic   string `json:"mnemonic,omitempty"`
}

func fromKey(priv *ecdsa.PrivateKey) *Wallet {
	pub := core.CompressPubKey(&priv.PublicKey)
	pkh := core.HashPubKey(pub)
	return &Wallet{priv: priv, pub: pub, pkh: pkh, addr: core.AddressFromPKH(pkh)}
}

// New gera uma chave nova (crypto/rand, sempre) e a salva em path com 0600.
// Arquivo existente é erro: sobrescrever uma wallet destruiria a chave — e
// com ela os fundos — sem volta. Prefira NewWithMnemonic: mesma segurança,
// com backup humano de 12 palavras.
func New(path string) (*Wallet, error) {
	priv, err := core.GenerateKey()
	if err != nil {
		return nil, err
	}
	return saveKey(path, priv, "")
}

// saveKey serializa a chave (PKCS8) e grava o wallet.json com as proteções
// de sempre: 0600 e O_EXCL (nunca sobrescreve). mnemonic vazio = wallet de
// chave aleatória (sem frase).
func saveKey(path string, priv *ecdsa.PrivateKey, mnemonic string) (*Wallet, error) {
	w := fromKey(priv)
	w.mnemonic = mnemonic
	blob, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(walletFile{
		PrivateKey: base64.StdEncoding.EncodeToString(blob),
		PublicKey:  hex.EncodeToString(w.pub),
		Address:    w.addr,
		Mnemonic:   mnemonic,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrWalletExists, path)
		}
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return nil, err
	}
	return w, nil
}

// Load lê e valida a wallet: permissão restrita, chave P-256, e a pública e
// o endereço do arquivo têm que ser deriváveis da privada (qualquer edição
// manual desalinhada aparece como ErrCorrupt, não como fundos sumidos).
func Load(path string) (*Wallet, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return nil, fmt.Errorf("%w: %s está com %04o (rode: chmod 600 %s)", ErrBadPerm, path, perm, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf walletFile
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	blob, err := base64.StdEncoding.DecodeString(wf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: private_key não é base64", ErrCorrupt)
	}
	key, err := x509.ParsePKCS8PrivateKey(blob)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	priv, ok := key.(*ecdsa.PrivateKey)
	if !ok || priv.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%w: chave não é ECDSA P-256", ErrCorrupt)
	}
	w := fromKey(priv)
	if wf.PublicKey != hex.EncodeToString(w.pub) || wf.Address != w.addr {
		return nil, fmt.Errorf("%w: pública/endereço não derivam da chave privada", ErrCorrupt)
	}
	if wf.Mnemonic != "" {
		// A frase gravada TEM que derivar exatamente esta chave — uma frase
		// trocada no arquivo enganaria o dono na hora de recuperar.
		mpriv, mnemonic, err := keyFromMnemonic(wf.Mnemonic)
		if err != nil || mpriv.D.Cmp(priv.D) != 0 {
			return nil, fmt.Errorf("%w: as 12 palavras do arquivo não derivam a chave privada", ErrCorrupt)
		}
		w.mnemonic = mnemonic
	}
	return w, nil
}

func (w *Wallet) Address() string      { return w.addr }
func (w *Wallet) PubKeyHash() [20]byte { return w.pkh }
func (w *Wallet) PubKey() []byte       { return w.pub }

// Mnemonic devolve as 12 palavras da wallet ("" se ela nasceu de chave
// aleatória, antes do backup por frase existir).
func (w *Wallet) Mnemonic() string { return w.mnemonic }

package main

import (
	"strings"
	"sync"
)

// logStore é um io.Writer que guarda as últimas linhas do log do node
// embutido para a aba Atividade — a GUI lê snapshots; o terminal continua
// recebendo tudo via io.MultiWriter.
type logStore struct {
	mu      sync.Mutex
	lines   []string
	partial string
	max     int
	dirty   bool
}

func newLogStore(max int) *logStore { return &logStore{max: max} }

func (l *logStore) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial += string(p)
	for {
		nl := strings.IndexByte(l.partial, '\n')
		if nl < 0 {
			break
		}
		l.lines = append(l.lines, l.partial[:nl])
		l.partial = l.partial[nl+1:]
	}
	if over := len(l.lines) - l.max; over > 0 {
		l.lines = append([]string(nil), l.lines[over:]...)
	}
	l.dirty = true
	return len(p), nil
}

// Snapshot devolve o texto atual e se mudou desde a última leitura — a aba
// só re-renderiza quando há novidade.
func (l *logStore) Snapshot() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := l.dirty
	l.dirty = false
	return strings.Join(l.lines, "\n"), changed
}

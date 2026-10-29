package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Route é uma rota CIDR: Addr/Len -> interface de índice Iface.
type Route struct {
	Addr  uint32
	Len   uint8
	Iface uint16
}

// Table guarda as rotas (sem duplicatas) e os nomes das interfaces.
type Table struct {
	Routes []Route
	Ifaces []string
}

// Match é o resultado de uma consulta LPM.
type Match struct {
	Addr  uint32
	Len   uint8
	Iface uint16
}

func lenMask(l uint8) uint32 {
	if l == 0 {
		return 0
	}
	return ^uint32(0) << (32 - uint(l))
}

// ParseIPv4 converte "a.b.c.d" em uint32 sem alocar.
func ParseIPv4(s string) (uint32, bool) {
	var v uint32
	part, digits, dots := 0, 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			part = part*10 + int(c-'0')
			digits++
			if part > 255 || digits > 3 {
				return 0, false
			}
		case c == '.':
			if digits == 0 {
				return 0, false
			}
			v = v<<8 | uint32(part)
			part, digits = 0, 0
			dots++
		default:
			return 0, false
		}
	}
	if dots != 3 || digits == 0 {
		return 0, false
	}
	return v<<8 | uint32(part), true
}

func FormatIPv4(a uint32) string {
	return strconv.Itoa(int(a>>24)) + "." + strconv.Itoa(int(a>>16&255)) + "." +
		strconv.Itoa(int(a>>8&255)) + "." + strconv.Itoa(int(a&255))
}

// LoadRoutes lê o CSV "prefixo,tamanho,interface" (cabeçalho opcional).
// Bits de host do prefixo são zerados; duplicatas mantêm a última ocorrência.
func LoadRoutes(path string) (*Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	t := &Table{}
	ifaceID := map[string]uint16{}
	index := map[uint64]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for ln := 1; sc.Scan(); ln++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fs := strings.Split(line, ",")
		if len(fs) != 3 {
			return nil, fmt.Errorf("%s:%d: esperado prefixo,tamanho,interface", path, ln)
		}
		addr, ok := ParseIPv4(strings.TrimSpace(fs[0]))
		if !ok {
			if ln == 1 { // cabeçalho
				continue
			}
			return nil, fmt.Errorf("%s:%d: prefixo inválido %q", path, ln, fs[0])
		}
		l, err := strconv.Atoi(strings.TrimSpace(fs[1]))
		if err != nil || l < 0 || l > 32 {
			return nil, fmt.Errorf("%s:%d: tamanho inválido %q", path, ln, fs[1])
		}
		name := strings.TrimSpace(fs[2])
		id, ok := ifaceID[name]
		if !ok {
			id = uint16(len(t.Ifaces))
			ifaceID[name] = id
			t.Ifaces = append(t.Ifaces, name)
		}
		r := Route{Addr: addr & lenMask(uint8(l)), Len: uint8(l), Iface: id}
		k := uint64(r.Addr)<<8 | uint64(r.Len)
		if i, dup := index[k]; dup {
			t.Routes[i] = r
			continue
		}
		index[k] = len(t.Routes)
		t.Routes = append(t.Routes, r)
	}
	return t, sc.Err()
}

// LoadQueries lê um endereço IPv4 por linha.
func LoadQueries(path string) ([]uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var qs []uint32
	sc := bufio.NewScanner(f)
	for ln := 1; sc.Scan(); ln++ {
		s := strings.TrimSpace(sc.Text())
		if s == "" {
			continue
		}
		a, ok := ParseIPv4(s)
		if !ok {
			return nil, fmt.Errorf("%s:%d: endereço inválido %q", path, ln, s)
		}
		qs = append(qs, a)
	}
	return qs, sc.Err()
}

// Lookuper é a interface comum das tabelas de roteamento.
type Lookuper interface {
	Lookup(addr uint32) (Match, bool)
	// BatchLookup consulta todos os endereços e devolve um checksum
	// (usado no benchmark para evitar eliminação de código morto).
	BatchLookup(addrs []uint32) uint64
	// Depth devolve quantos nós foram visitados na consulta.
	Depth(addr uint32) int
}

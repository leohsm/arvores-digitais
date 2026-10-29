package main

import (
	"math/rand/v2"
	"os"
	"testing"
)

func checkAgainstRef(t *testing.T, name string, routes []Route, tree Lookuper, qs []uint32) {
	t.Helper()
	ref := BuildHashLPM(routes)
	for _, a := range qs {
		want, wok := ref.Lookup(a)
		got, gok := tree.Lookup(a)
		if want != got || wok != gok {
			t.Fatalf("%s: %s: esperado %v/%v, obtido %v/%v", name, FormatIPv4(a), want, wok, got, gok)
		}
	}
}

// Exemplo do enunciado.
func TestExemploEnunciado(t *testing.T) {
	spec := []struct {
		a string
		l uint8
		i uint16
	}{{"0.0.0.0", 0, 0}, {"10.0.0.0", 8, 1}, {"10.1.0.0", 16, 2}, {"10.1.2.0", 24, 3}, {"192.168.0.0", 16, 4}, {"192.168.10.0", 24, 5}}
	var rs []Route
	for _, s := range spec {
		a, _ := ParseIPv4(s.a)
		rs = append(rs, Route{a, s.l, s.i})
	}
	lc, err := BuildLCTrie(rs, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]uint16{"10.1.2.55": 3, "10.1.9.9": 2, "10.200.0.1": 1, "192.168.10.7": 5, "192.168.20.7": 4, "8.8.8.8": 0}
	for _, tree := range []Lookuper{BuildPatricia(rs), lc} {
		for ip, ifc := range want {
			a, _ := ParseIPv4(ip)
			m, ok := tree.Lookup(a)
			if !ok || m.Iface != ifc {
				t.Errorf("%T %s: obtido %v/%v, esperado eth%d", tree, ip, m, ok, ifc)
			}
		}
	}
}

// Tabelas aleatórias com muitos prefixos aninhados, buracos e sem rota padrão.
func TestAleatorio(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for iter := 0; iter < 300; iter++ {
		n := 1 + rng.IntN(300)
		seen := map[uint64]bool{}
		var rs []Route
		space := uint32(1) << (4 + rng.UintN(20)) // concentra endereços para forçar aninhamento
		for len(rs) < n {
			l := uint8(rng.IntN(33))
			if rng.IntN(3) == 0 {
				l = uint8(8 + rng.IntN(5))
			}
			a := (rng.Uint32() % space) << rng.UintN(8) & lenMask(l)
			if k := uint64(a)<<8 | uint64(l); !seen[k] {
				seen[k] = true
				rs = append(rs, Route{a, l, uint16(rng.IntN(8))})
			}
			if len(seen) > 1<<12 {
				break
			}
		}
		qs := make([]uint32, 2000)
		for i := range qs {
			if i%2 == 0 {
				r := rs[rng.IntN(len(rs))]
				qs[i] = r.Addr | rng.Uint32()&^lenMask(r.Len)
			} else {
				qs[i] = (rng.Uint32() % space) << rng.UintN(8)
			}
		}
		checkAgainstRef(t, "patricia", rs, BuildPatricia(rs), qs)
		for _, f := range []float64{1, 0.5, 0.25} {
			lc, err := BuildLCTrie(rs, f)
			if err != nil {
				t.Fatal(err)
			}
			checkAgainstRef(t, "lctrie", rs, lc, qs)
		}
	}
}

// Tabela BGP completa, se o arquivo estiver ao lado do projeto.
func TestTabelaCompleta(t *testing.T) {
	path := "../rib.20260701.0000.rotas_ipv4.csv"
	if _, err := os.Stat(path); err != nil {
		t.Skip("CSV não encontrado")
	}
	tab, err := LoadRoutes(path)
	if err != nil {
		t.Fatal(err)
	}
	qs := append(GenQueries(tab.Routes, 300000, "rotas", 3), GenQueries(tab.Routes, 300000, "aleatorio", 4)...)
	checkAgainstRef(t, "patricia", tab.Routes, BuildPatricia(tab.Routes), qs)
	for _, f := range []float64{1, 0.5, 0.25} {
		lc, err := BuildLCTrie(tab.Routes, f)
		if err != nil {
			t.Fatal(err)
		}
		checkAgainstRef(t, "lctrie", tab.Routes, lc, qs)
	}
}

func TestParseIPv4(t *testing.T) {
	for _, s := range []string{"1.2.3", "1.2.3.4.5", "256.0.0.0", "a.b.c.d", "1..2.3", ""} {
		if _, ok := ParseIPv4(s); ok {
			t.Errorf("%q deveria ser inválido", s)
		}
	}
	if a, ok := ParseIPv4("10.1.2.55"); !ok || FormatIPv4(a) != "10.1.2.55" {
		t.Error("ida e volta falhou")
	}
}

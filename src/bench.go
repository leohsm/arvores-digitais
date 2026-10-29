package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sort"
	"time"
)

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

type structure struct {
	name    string
	build   func() (Lookuper, error)
	bytes   func(Lookuper) int
	extra   func(Lookuper) string
	buildMs float64
	heapMB  float64
	tree    Lookuper
}

func cmdBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	rotas := fs.String("rotas", "rotas.csv", "arquivo de rotas")
	n := fs.Int("n", 2000000, "consultas por trace")
	reps := fs.Int("reps", 7, "repetições de cada medição (usa a mediana)")
	fs.Parse(args)
	runtime.LockOSThread()

	t0 := time.Now()
	tab, err := LoadRoutes(*rotas)
	if err != nil {
		return err
	}
	routes := tab.Routes
	fmt.Printf("# Rotas: %d | interfaces: %d | leitura+parse do CSV: %.0f ms\n", len(routes), len(tab.Ifaces), since(t0))
	fmt.Printf("# Go %s %s/%s, %d CPUs, consultas por trace: %d, repetições: %d (mediana)\n\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), *n, *reps)

	var sts []*structure
	sts = append(sts, &structure{
		name:  "Patricia",
		build: func() (Lookuper, error) { return BuildPatricia(routes), nil },
		bytes: func(l Lookuper) int { return l.(*Patricia).Bytes() },
		extra: func(l Lookuper) string {
			tot, rt := l.(*Patricia).NodeCount()
			return fmt.Sprintf("%d nós (%d com rota, %d de junção)", tot, rt, tot-rt)
		},
	})
	for _, fill := range []float64{1.0, 0.75, 0.5, 0.25} {
		fill := fill
		sts = append(sts, &structure{
			name:  fmt.Sprintf("LC-Trie f=%.2f", fill),
			build: func() (Lookuper, error) { return BuildLCTrie(routes, fill) },
			bytes: func(l Lookuper) int { return l.(*LCTrie).Bytes() },
			extra: func(l Lookuper) string {
				c := l.(*LCTrie)
				return fmt.Sprintf("trie=%d nós, base=%d (%d sintéticas), pv=%d", len(c.trie), len(c.base), c.Synthetic, len(c.pv))
			},
		})
	}
	sts = append(sts, &structure{
		name:  "Hash por tamanho",
		build: func() (Lookuper, error) { return BuildHashLPM(routes), nil },
		bytes: func(Lookuper) int { return 0 },
		extra: func(Lookuper) string { return "baseline (maps do Go; memória só pelo heap)" },
	})

	// Construção: mediana de `reps` builds; memória do heap com 1 build retido.
	for _, s := range sts {
		var ts []float64
		for i := 0; i < *reps; i++ {
			before := heapInUse()
			t := time.Now()
			tr, err := s.build()
			if err != nil {
				return fmt.Errorf("%s: %w", s.name, err)
			}
			ts = append(ts, since(t))
			s.heapMB = float64(heapInUse()-before) / (1 << 20)
			s.tree = tr
		}
		s.buildMs = median(ts)
	}

	fmt.Println("## Construção e memória")
	fmt.Printf("%-18s %12s %14s %12s  %s\n", "estrutura", "build (ms)", "estrutural(MB)", "heap (MB)", "detalhes")
	for _, s := range sts {
		fmt.Printf("%-18s %12.1f %14.1f %12.1f  %s\n", s.name, s.buildMs, float64(s.bytes(s.tree))/(1<<20), s.heapMB, s.extra(s.tree))
	}

	traces := []struct{ nome, modo string }{
		{"rotas (endereços dentro de rotas da tabela)", "rotas"},
		{"aleatorio (32 bits uniformes)", "aleatorio"},
		{"misto (50/50)", "misto"},
	}
	ref := BuildHashLPM(routes)
	for _, tr := range traces {
		qs := GenQueries(routes, *n, tr.modo, 42)
		want := ref.BatchLookup(qs)
		fmt.Printf("\n## Consulta - trace %s\n", tr.nome)
		fmt.Printf("%-18s %10s %12s %12s %10s %6s\n", "estrutura", "ns/cons.", "Mcons/s", "prof. média", "prof. máx", "ok")
		var patNs float64
		for i, s := range sts {
			var ts []float64
			var sum uint64
			for r := 0; r < *reps; r++ {
				t := time.Now()
				sum = s.tree.BatchLookup(qs)
				ts = append(ts, float64(time.Since(t).Nanoseconds())/float64(len(qs)))
			}
			ns := median(ts)
			if i == 0 {
				patNs = ns
			}
			var tot, mx int
			for _, a := range qs[:min(len(qs), 200000)] {
				d := s.tree.Depth(a)
				tot += d
				mx = max(mx, d)
			}
			ok := "sim"
			if sum != want {
				ok = "NÃO"
			}
			fmt.Printf("%-18s %10.1f %12.2f %12.2f %10d %6s", s.name, ns, 1e3/ns, float64(tot)/float64(min(len(qs), 200000)), mx, ok)
			if i > 0 {
				fmt.Printf("   (%.2fx vs Patricia)", patNs/ns)
			}
			fmt.Println()
		}
	}

	// Atualização: inserir k rotas novas em Patricia vs reconstruir a LC-Trie.
	fmt.Println("\n## Atualização incremental")
	const k = 10000
	rng := rand.New(rand.NewPCG(7, 7))
	have := map[uint64]bool{}
	for _, r := range routes {
		have[uint64(r.Addr)<<8|uint64(r.Len)] = true
	}
	var novas []Route
	for len(novas) < k {
		r := Route{Addr: rng.Uint32() & lenMask(24), Len: 24, Iface: 0}
		if !have[uint64(r.Addr)<<8|24] {
			have[uint64(r.Addr)<<8|24] = true
			novas = append(novas, r)
		}
	}
	p := BuildPatricia(routes)
	t1 := time.Now()
	for _, r := range novas {
		p.Insert(r)
	}
	insMs := since(t1)
	fmt.Printf("Patricia: %d inserções incrementais em %.2f ms (%.0f ns/inserção)\n", k, insMs, insMs*1e6/k)
	all := append(append([]Route(nil), routes...), novas...)
	t2 := time.Now()
	if _, err := BuildLCTrie(all, 0.5); err != nil {
		return err
	}
	fmt.Printf("LC-Trie : 1 atualização exige reconstrução completa: %.1f ms\n", since(t2))
	return nil
}

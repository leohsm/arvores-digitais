package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"time"
)

const usage = `uso: arvoresdigitais <comando> [opções]

  rotear  -rotas rotas.csv -consultas consultas.txt -saida saida.txt [-arvore patricia|lctrie] [-fill 0.5]
  gerar   -rotas rotas.csv -saida consultas.txt [-n 1000000] [-modo rotas|aleatorio|misto] [-seed 1]
  bench   -rotas rotas.csv [-n 2000000] [-reps 7]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "rotear":
		err = cmdRotear(os.Args[2:])
	case "gerar":
		err = cmdGerar(os.Args[2:])
	case "bench":
		err = cmdBench(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func buildTree(name string, routes []Route, fill float64) (Lookuper, error) {
	switch name {
	case "patricia":
		return BuildPatricia(routes), nil
	case "lctrie":
		return BuildLCTrie(routes, fill)
	case "hash":
		return BuildHashLPM(routes), nil
	}
	return nil, fmt.Errorf("árvore desconhecida %q (use patricia, lctrie ou hash)", name)
}

// rotear: lê rotas e consultas, grava "endereço interface prefixo_vencedor".
func cmdRotear(args []string) error {
	fs := flag.NewFlagSet("rotear", flag.ExitOnError)
	rotas := fs.String("rotas", "rotas.csv", "arquivo de rotas")
	cons := fs.String("consultas", "consultas.txt", "arquivo de consultas")
	saida := fs.String("saida", "saida.txt", "arquivo de saída")
	arv := fs.String("arvore", "lctrie", "patricia | lctrie | hash")
	fill := fs.Float64("fill", 0.5, "fator de preenchimento da LC-Trie")
	fs.Parse(args)

	t, err := LoadRoutes(*rotas)
	if err != nil {
		return err
	}
	qs, err := LoadQueries(*cons)
	if err != nil {
		return err
	}
	tree, err := buildTree(*arv, t.Routes, *fill)
	if err != nil {
		return err
	}
	f, err := os.Create(*saida)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	buf := make([]byte, 0, 96)
	for _, a := range qs {
		buf = append(buf[:0], FormatIPv4(a)...)
		if m, ok := tree.Lookup(a); ok {
			buf = append(buf, ' ')
			buf = append(buf, t.Ifaces[m.Iface]...)
			buf = append(buf, ' ')
			buf = append(buf, FormatIPv4(m.Addr)...)
			buf = append(buf, '/')
			buf = strconv.AppendInt(buf, int64(m.Len), 10)
		} else {
			buf = append(buf, " descartado"...)
		}
		buf = append(buf, '\n')
		w.Write(buf)
	}
	return w.Flush()
}

// GenQueries cria consultas: "rotas" sorteia um endereço dentro de uma rota
// sorteada (tráfego que casa com a tabela); "aleatorio" sorteia 32 bits
// uniformes; "misto" é metade/metade.
func GenQueries(routes []Route, n int, modo string, seed uint64) []uint32 {
	rng := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	qs := make([]uint32, n)
	for i := range qs {
		useRoute := modo == "rotas" || (modo == "misto" && i%2 == 0)
		if useRoute && len(routes) > 0 {
			r := routes[rng.IntN(len(routes))]
			qs[i] = r.Addr | rng.Uint32()&^lenMask(r.Len)
		} else {
			qs[i] = rng.Uint32()
		}
	}
	if modo == "misto" {
		rng.Shuffle(n, func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
	}
	return qs
}

func cmdGerar(args []string) error {
	fs := flag.NewFlagSet("gerar", flag.ExitOnError)
	rotas := fs.String("rotas", "rotas.csv", "arquivo de rotas")
	saida := fs.String("saida", "consultas.txt", "arquivo de consultas")
	n := fs.Int("n", 1000000, "número de consultas")
	modo := fs.String("modo", "misto", "rotas | aleatorio | misto")
	seed := fs.Uint64("seed", 1, "semente")
	fs.Parse(args)

	t, err := LoadRoutes(*rotas)
	if err != nil {
		return err
	}
	f, err := os.Create(*saida)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	for _, a := range GenQueries(t.Routes, *n, *modo, *seed) {
		w.WriteString(FormatIPv4(a))
		w.WriteByte('\n')
	}
	return w.Flush()
}

func since(t time.Time) float64 { return float64(time.Since(t).Nanoseconds()) / 1e6 } // ms

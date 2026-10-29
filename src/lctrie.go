package main

import (
	"errors"
	"math/bits"
	"sort"
	"unsafe"
)

// LC-Trie (Nilsson & Karlsson, 1999): árvore com compressão de caminho (skip)
// e de nível (branch). Cada nó interno testa `branch` bits de uma vez e tem
// 2^branch filhos contíguos no vetor trie. Folhas apontam para o vetor base.
//
// Como a LC-Trie exige um conjunto livre de prefixos, as rotas que são prefixo
// de outras ("internas") ficam no vetor pv; cada entrada (folha ou interna)
// guarda `pre`, o índice em pv da rota interna mais longa que a cobre.
//
// Nó (uint32): branch(5 bits) | skip(5 bits) | adr(22 bits).
const (
	adrBits   = 22
	adrMask   = 1<<adrBits - 1
	maxBranch = 24
)

type lcEntry struct {
	addr, mask uint32
	pre        int32
	iface      uint16
	plen       uint8
}

type LCTrie struct {
	trie []uint32
	base []lcEntry // folhas (prefix-free) + folhas sintéticas de buckets vazios
	pv   []lcEntry // rotas que são prefixo de outras

	Fill      float64
	Synthetic int // folhas sintéticas criadas para buckets vazios
}

func pack(branch, skip, adr uint32) uint32 { return branch<<27 | skip<<adrBits | adr }

// BuildLCTrie constrói a LC-Trie. fill é o fator de preenchimento (0<fill<=1):
// um nó aumenta o branch enquanto pelo menos fill*2^branch filhos forem não-vazios.
func BuildLCTrie(routes []Route, fill float64) (*LCTrie, error) {
	es := make([]lcEntry, len(routes))
	for i, r := range routes {
		es[i] = lcEntry{addr: r.Addr, mask: lenMask(r.Len), iface: r.Iface, plen: r.Len}
	}
	sort.Slice(es, func(i, j int) bool {
		if es[i].addr != es[j].addr {
			return es[i].addr < es[j].addr
		}
		return es[i].plen < es[j].plen
	})

	b := &lcBuilder{t: &LCTrie{Fill: fill}, fill: fill}
	// Separa internas (cobrem a próxima entrada na ordem) de folhas.
	// Na ordem (addr, plen), as cobertas por P vêm logo depois dele.
	var stack []int32
	for i := range es {
		e := es[i]
		for len(stack) > 0 {
			top := b.t.pv[stack[len(stack)-1]]
			if e.addr&top.mask == top.addr {
				break
			}
			stack = stack[:len(stack)-1]
		}
		e.pre = -1
		if len(stack) > 0 {
			e.pre = stack[len(stack)-1]
		}
		if i+1 < len(es) && es[i+1].addr&e.mask == e.addr {
			b.t.pv = append(b.t.pv, e)
			stack = append(stack, int32(len(b.t.pv)-1))
		} else {
			b.leaves = append(b.leaves, e)
		}
	}
	if len(b.leaves) == 0 {
		return b.t, nil
	}
	b.t.trie = make([]uint32, 1, 2*len(b.leaves))
	b.t.base = make([]lcEntry, 0, len(b.leaves))
	b.build(0, len(b.leaves), 0, 0)
	if b.err != nil {
		return nil, b.err
	}
	return b.t, nil
}

type lcBuilder struct {
	t      *LCTrie
	leaves []lcEntry
	fill   float64
	err    error
}

func extract(pos, branch uint32, a uint32) uint32 { return (a << pos) >> (32 - branch) }

func (b *lcBuilder) build(lo, hi int, pos uint32, slot int) {
	if hi-lo == 1 {
		b.t.base = append(b.t.base, b.leaves[lo])
		b.setNode(slot, pack(0, 0, uint32(len(b.t.base)-1)))
		return
	}
	first, last := b.leaves[lo].addr, b.leaves[hi-1].addr
	pos2 := uint32(bits.LeadingZeros32(first ^ last)) // prefixo comum de todo o intervalo
	skip := pos2 - pos

	minLen := uint32(32)
	for i := lo; i < hi; i++ {
		minLen = min(minLen, uint32(b.leaves[i].plen))
	}
	limit := min(minLen-pos2, maxBranch) // todo prefixo precisa ter os bits testados
	br := uint32(1)
	for c := uint32(2); c <= limit; c++ {
		distinct, prev := 0, uint32(1<<31) // valor impossível para c<=24
		for i := lo; i < hi; i++ {
			v := extract(pos2, c, b.leaves[i].addr)
			if v != prev {
				distinct++
				prev = v
			}
		}
		if float64(distinct) < b.fill*float64(uint64(1)<<c) {
			break
		}
		br = c
	}

	first_child := len(b.t.trie)
	if first_child+(1<<br) > adrMask {
		b.err = errors.New("lctrie: vetor trie excede o endereçamento de 22 bits")
		return
	}
	b.t.trie = append(b.t.trie, make([]uint32, 1<<br)...)
	b.setNode(slot, pack(br, skip, uint32(first_child)))

	i := lo
	for pat := uint32(0); pat < 1<<br && b.err == nil; pat++ {
		j := i
		for j < hi && extract(pos2, br, b.leaves[j].addr) == pat {
			j++
		}
		if j > i {
			b.build(i, j, pos2+br, first_child+int(pat))
		} else {
			b.synthetic(first, pos2, br, pat, first_child+int(pat))
		}
		i = j
	}
}

func (b *lcBuilder) setNode(slot int, v uint32) { b.t.trie[slot] = v }

// synthetic cria uma folha que nunca casa por si só, mas cuja cadeia `pre`
// aponta para a rota interna que cobre o bucket vazio (ex.: rota padrão).
func (b *lcBuilder) synthetic(first uint32, pos2, br, pat uint32, slot int) {
	regionLen := pos2 + br
	A := (first &^ (^uint32(0) >> pos2)) | pat<<(32-regionLen)
	pre := b.coverOf(A, uint8(regionLen))
	b.t.base = append(b.t.base, lcEntry{addr: 1, mask: 0, pre: pre}) // addr&mask=0 != 1
	b.t.Synthetic++
	b.setNode(slot, pack(0, 0, uint32(len(b.t.base)-1)))
}

// coverOf: rota interna mais longa, com len<=regionLen, que cobre a região A.
// Toda rota interna tem folhas sob ela, logo está na cadeia `pre` da folha
// imediatamente anterior ou posterior a A.
func (b *lcBuilder) coverOf(A uint32, regionLen uint8) int32 {
	k := sort.Search(len(b.leaves), func(i int) bool { return b.leaves[i].addr >= A })
	best := int32(-1)
	for _, i := range [2]int{k - 1, k} {
		if i < 0 || i >= len(b.leaves) {
			continue
		}
		for p := b.leaves[i].pre; p >= 0; p = b.t.pv[p].pre {
			e := &b.t.pv[p]
			if e.plen <= regionLen && A&e.mask == e.addr {
				if best < 0 || e.plen > b.t.pv[best].plen {
					best = p
				}
				break
			}
		}
	}
	return best
}

func (l *LCTrie) find(a uint32) *lcEntry {
	if len(l.trie) == 0 {
		return nil
	}
	nd := l.trie[0]
	pos := (nd >> adrBits) & 31
	branch := nd >> 27
	adr := nd & adrMask
	for branch != 0 {
		nd = l.trie[adr+(a<<pos)>>(32-branch)]
		pos += branch + (nd>>adrBits)&31
		branch = nd >> 27
		adr = nd & adrMask
	}
	e := &l.base[adr]
	if a&e.mask == e.addr {
		return e
	}
	for p := e.pre; p >= 0; {
		q := &l.pv[p]
		if a&q.mask == q.addr {
			return q
		}
		p = q.pre
	}
	return nil
}

func (l *LCTrie) Lookup(a uint32) (Match, bool) {
	e := l.find(a)
	if e == nil {
		return Match{}, false
	}
	return Match{e.addr, e.plen, e.iface}, true
}

func (l *LCTrie) BatchLookup(addrs []uint32) uint64 {
	var sum uint64
	for _, a := range addrs {
		if e := l.find(a); e != nil {
			sum += uint64(e.iface) + uint64(e.plen) + 1
		}
	}
	return sum
}

// Depth: nós da trie visitados (cada um é um acesso a memória), mais a folha.
func (l *LCTrie) Depth(a uint32) int {
	if len(l.trie) == 0 {
		return 0
	}
	d := 1
	nd := l.trie[0]
	pos := (nd >> adrBits) & 31
	branch := nd >> 27
	adr := nd & adrMask
	for branch != 0 {
		nd = l.trie[adr+(a<<pos)>>(32-branch)]
		d++
		pos += branch + (nd>>adrBits)&31
		branch = nd >> 27
		adr = nd & adrMask
	}
	return d
}

func (l *LCTrie) Bytes() int {
	es := int(unsafe.Sizeof(lcEntry{}))
	return len(l.trie)*4 + (len(l.base)+len(l.pv))*es
}

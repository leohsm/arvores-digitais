package main

import (
	"math/bits"
	"unsafe"
)

// Patricia: árvore digital binária com compressão de caminho. Cada nó guarda
// o prefixo completo (key/plen) que o identifica e dois filhos; sequências de
// bits sem bifurcação são puladas. Nós internos sem rota ("nós de junção")
// sempre têm dois filhos, então há no máximo ~2n nós para n rotas.
type pNode struct {
	key      uint32
	child    [2]int32
	iface    uint16
	plen     uint8
	hasRoute bool
}

type Patricia struct {
	nodes []pNode
	root  int32
}

func NewPatricia() *Patricia { return &Patricia{root: -1} }

func BuildPatricia(routes []Route) *Patricia {
	p := NewPatricia()
	p.nodes = make([]pNode, 0, 2*len(routes))
	for _, r := range routes {
		p.Insert(r)
	}
	return p
}

func bitAt(key uint32, i uint8) int { return int(key>>(31-uint(i))) & 1 }

// commonLen: tamanho do prefixo comum entre (k1,l1) e (k2,l2).
func commonLen(k1 uint32, l1 uint8, k2 uint32, l2 uint8) uint8 {
	c := uint8(bits.LeadingZeros32(k1 ^ k2))
	return min(c, l1, l2)
}

func (p *Patricia) newNode(n pNode) int32 {
	p.nodes = append(p.nodes, n)
	return int32(len(p.nodes) - 1)
}

// Insert insere (ou atualiza) uma rota. Custo O(32) no pior caso.
func (p *Patricia) Insert(r Route) {
	key := r.Addr & lenMask(r.Len)
	leaf := pNode{key: key, plen: r.Len, hasRoute: true, iface: r.Iface, child: [2]int32{-1, -1}}

	parent, side := int32(-1), 0
	cur := p.root
	for {
		if cur < 0 {
			p.link(parent, side, p.newNode(leaf))
			return
		}
		n := p.nodes[cur]
		c := commonLen(n.key, n.plen, key, r.Len)
		switch {
		case c == n.plen && c == r.Len: // mesmo prefixo
			p.nodes[cur].hasRoute = true
			p.nodes[cur].iface = r.Iface
			return
		case c == n.plen: // n é prefixo da nova rota: desce
			parent, side = cur, bitAt(key, n.plen)
			cur = n.child[side]
		case c == r.Len: // nova rota é prefixo de n: entra acima de n
			leaf.child[bitAt(n.key, r.Len)] = cur
			p.link(parent, side, p.newNode(leaf))
			return
		default: // divergem em c: cria nó de junção
			j := pNode{key: key & lenMask(c), plen: c, child: [2]int32{-1, -1}}
			j.child[bitAt(n.key, c)] = cur
			j.child[bitAt(key, c)] = p.newNode(leaf)
			p.link(parent, side, p.newNode(j))
			return
		}
	}
}

func (p *Patricia) link(parent int32, side int, child int32) {
	if parent < 0 {
		p.root = child
	} else {
		p.nodes[parent].child[side] = child
	}
}

// find devolve o índice do nó com a melhor rota (prefixo mais longo) ou -1.
func (p *Patricia) find(a uint32) int32 {
	best := int32(-1)
	n := p.root
	for n >= 0 {
		nd := &p.nodes[n]
		if (a^nd.key)>>(32-uint(nd.plen)) != 0 { // 32-plen=32 => resultado 0 em Go
			break
		}
		if nd.hasRoute {
			best = n
		}
		if nd.plen == 32 {
			break
		}
		n = nd.child[(a>>(31-uint(nd.plen)))&1]
	}
	return best
}

func (p *Patricia) Lookup(a uint32) (Match, bool) {
	i := p.find(a)
	if i < 0 {
		return Match{}, false
	}
	nd := &p.nodes[i]
	return Match{nd.key, nd.plen, nd.iface}, true
}

func (p *Patricia) BatchLookup(addrs []uint32) uint64 {
	var sum uint64
	for _, a := range addrs {
		if i := p.find(a); i >= 0 {
			nd := &p.nodes[i]
			sum += uint64(nd.iface) + uint64(nd.plen) + 1
		}
	}
	return sum
}

func (p *Patricia) Depth(a uint32) int {
	d := 0
	n := p.root
	for n >= 0 {
		d++
		nd := &p.nodes[n]
		if (a^nd.key)>>(32-uint(nd.plen)) != 0 || nd.plen == 32 {
			break
		}
		n = nd.child[(a>>(31-uint(nd.plen)))&1]
	}
	return d
}

// Bytes: memória estrutural dos nós.
func (p *Patricia) Bytes() int { return len(p.nodes) * int(unsafe.Sizeof(pNode{})) }

func (p *Patricia) NodeCount() (total, withRoute int) {
	for i := range p.nodes {
		if p.nodes[i].hasRoute {
			withRoute++
		}
	}
	return len(p.nodes), withRoute
}

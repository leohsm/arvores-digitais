package main

// HashLPM é a implementação de referência: uma tabela hash por tamanho de
// prefixo, testada do /32 ao /0. Serve de oráculo nos testes e de linha de base
// no benchmark; não é uma árvore digital.
type HashLPM struct {
	byLen [33]map[uint32]uint16
	lens  []uint8 // tamanhos presentes, em ordem decrescente
}

func BuildHashLPM(routes []Route) *HashLPM {
	h := &HashLPM{}
	for _, r := range routes {
		if h.byLen[r.Len] == nil {
			h.byLen[r.Len] = map[uint32]uint16{}
		}
		h.byLen[r.Len][r.Addr&lenMask(r.Len)] = r.Iface
	}
	for l := 32; l >= 0; l-- {
		if h.byLen[l] != nil {
			h.lens = append(h.lens, uint8(l))
		}
	}
	return h
}

func (h *HashLPM) Lookup(a uint32) (Match, bool) {
	for _, l := range h.lens {
		k := a & lenMask(l)
		if ifc, ok := h.byLen[l][k]; ok {
			return Match{k, l, ifc}, true
		}
	}
	return Match{}, false
}

func (h *HashLPM) BatchLookup(addrs []uint32) uint64 {
	var sum uint64
	for _, a := range addrs {
		if m, ok := h.Lookup(a); ok {
			sum += uint64(m.Iface) + uint64(m.Len) + 1
		}
	}
	return sum
}

func (h *HashLPM) Depth(a uint32) int {
	d := 0
	for _, l := range h.lens {
		d++
		if _, ok := h.byLen[l][a&lenMask(l)]; ok {
			break
		}
	}
	return d
}

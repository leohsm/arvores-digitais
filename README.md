# Árvores Digitais: Patricia e LC-Trie no roteamento IPv4

UTFPR – Câmpus Medianeira | Ciência da Computação | Árvores e Grafos | 2026.2
Projeto Prático 3 – Árvores Digitais [Patricia e LC-Trie]

Equipe: Erik Mazzuco, Letícia Moro, Leonardo Herrero

Simulador de encaminhamento de pacotes IPv4 que escolhe a interface de saída pela
rota de maior prefixo coincidente (longest prefix match), com a tabela de rotas
implementada como Patricia e como LC-Trie.

## Estrutura

| Caminho | Conteúdo |
|---|---|
| `src/` | Código em Go (sem dependências externas) |
| `src/patricia.go` | Patricia (compressão de caminho) |
| `src/lctrie.go` | LC-Trie (compressão de caminho e de nível) |
| `src/ref.go` | Referência por tabela hash, usada como oráculo e linha de base |
| `src/trees_test.go` | Testes automatizados |
| `relatorio/relatorio_projeto3.pdf` | Relatório final |
| `relatorio/gerar_relatorio.js` | Gerador do relatório (.docx) |
| `relatorio/topdf.py` | Conversão .docx → PDF via LibreOffice |
| `resultados_bench.txt` | Saída do benchmark usada no relatório |

## Uso

Requer Go 1.25 ou mais recente. A partir de `src/`:

```
go test ./...
go build -o arvoresdigitais.exe .

arvoresdigitais.exe gerar  -rotas rotas.csv -saida consultas.txt [-n 1000000] [-modo rotas|aleatorio|misto]
arvoresdigitais.exe rotear -rotas rotas.csv -consultas consultas.txt -saida saida.txt [-arvore patricia|lctrie] [-fill 0.5]
arvoresdigitais.exe bench  -rotas rotas.csv [-n 2000000] [-reps 7]
```

`rotas.csv` tem uma rota por linha (`prefixo,tamanho,interface`). Cada linha de
`saida.txt` tem a forma `endereço interface prefixo_vencedor`, por exemplo
`10.1.9.9 eth2 10.1.0.0/16`; sem rota que case, `endereço descartado`.

A tabela BGP usada (`rib.20260701.0000.rotas_ipv4.csv`, RouteViews) está no
Moodle da disciplina e não é versionada. O teste `TestTabelaCompleta` a procura
na raiz do repositório e é pulado se ela não estiver lá.

## Gerar o relatório

```
cd relatorio
npm install docx@9
node gerar_relatorio.js relatorio_projeto3.docx
"C:\Program Files\LibreOffice\program\python.exe" topdf.py relatorio_projeto3.docx relatorio_projeto3.pdf
```

## Resultados principais (LC-Trie com fator de preenchimento 0,5)

| | Patricia | LC-Trie |
|---|---|---|
| Consulta (tráfego que casa com a tabela) | 522,7 ns | 112,1 ns (4,66x) |
| Consulta (tráfego aleatório) | 224,5 ns | 63,9 ns (3,51x) |
| Memória estrutural | 30,9 MB | 25,9 MB |
| Atualização | 655 ns por inserção | reconstrução completa, 475 ms |

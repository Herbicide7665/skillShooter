package diffutil

import (
	"fmt"
	"strings"
)

// Unified 生成两个文本的 unified diff（行级 LCS），context 为上下文行数
func Unified(oldName, newName, a, b string, context int) string {
	if context <= 0 {
		context = 3
	}
	al := strings.Split(strings.ReplaceAll(a, "\r\n", "\n"), "\n")
	bl := strings.Split(strings.ReplaceAll(b, "\r\n", "\n"), "\n")

	ops := diffOps(al, bl) // 序列: ('=',aIdx), ('-',aIdx), ('+',bIdx)
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", oldName, newName)

	// 聚合 hunk：把 close 的改动合并成一个 hunk
	i := 0
	for i < len(ops) {
		// 找下一个非'='
		if ops[i].op == '=' {
			i++
			continue
		}
		// hunk 起点：前 context 行
		start := i - context
		if start < 0 {
			start = 0
		}
		// 找 hunk 结尾：其后连续改动 + 中间间隔 <= 2*context 的改动段
		j := i
		gap := 0
		end := i
		for j < len(ops) {
			if ops[j].op == '=' {
				gap++
				if gap > 2*context {
					break
				}
			} else {
				gap = 0
				end = j
			}
			j++
		}
		hunkEnd := end + context
		if hunkEnd >= len(ops) {
			hunkEnd = len(ops) - 1
		}

		// 计算 a/b 行号范围
		aStart, bStart := -1, -1
		aCount, bCount := 0, 0
		for k := start; k <= hunkEnd; k++ {
			o := ops[k]
			switch o.op {
			case '=':
				if aStart < 0 {
					aStart = o.a
					bStart = o.b
				}
				aCount++
				bCount++
			case '-':
				if aStart < 0 {
					aStart = o.a
					if bStart < 0 {
						bStart = o.b2
					}
				}
				aCount++
			case '+':
				if aStart < 0 {
					aStart = o.a2
					if bStart < 0 {
						bStart = o.b
					}
				}
				bCount++
			}
		}
		if aStart < 0 {
			aStart = len(al)
		}
		if bStart < 0 {
			bStart = len(bl)
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart+1, aCount, bStart+1, bCount)
		for k := start; k <= hunkEnd; k++ {
			o := ops[k]
			switch o.op {
			case '=':
				fmt.Fprintf(&sb, " %s\n", al[o.a])
			case '-':
				fmt.Fprintf(&sb, "-%s\n", al[o.a])
			case '+':
				fmt.Fprintf(&sb, "+%s\n", bl[o.b])
			}
		}
		i = end + 1
	}
	return sb.String()
}

type op struct {
	op           byte
	a, b         int // '=' : a in al, b in bl ; '-' : a ; '+' : b
	a2, b2       int // 兜底行号
}

// diffOps 行级 LCS diff
func diffOps(al, bl []string) []op {
	n, m := len(al), len(bl)
	// 防爆内存：超大文件退化为简单替换
	if n*m > 4_000_000 {
		return append([]op{{op: '-', a: 0, a2: 0}}, []op{{op: '+', b: 0, b2: 0}}...)
	}
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		if al[i] == bl[j] {
			ops = append(ops, op{op: '=', a: i, b: j})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, op{op: '-', a: i, a2: i, b2: j})
			i++
		} else {
			ops = append(ops, op{op: '+', b: j, a2: i, b2: j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{op: '-', a: i, a2: i, b2: j})
	}
	for ; j < m; j++ {
		ops = append(ops, op{op: '+', b: j, a2: i, b2: j})
	}
	return ops
}

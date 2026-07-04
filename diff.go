package main

import "regexp"

// diffOp classifies a line in an edit script.
type diffOp int

const (
	opEqual diffOp = iota
	opDel
	opAdd
)

// diffLine is one line of a diff: an op and the line text.
type diffLine struct {
	op   diffOp
	text string
}

// diffLines computes a line-level shortest edit script turning a into b, via the Myers O(ND)
// algorithm (pure Go, no dependency). The returned ops, read top to bottom, are the diff:
// opEqual/opAdd lines reconstruct b; opEqual/opDel lines reconstruct a.
func diffLines(a, b []string) []diffLine {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	trace, endX, endY := myersTrace(a, b)
	return myersBacktrack(a, b, trace, endX, endY)
}

// myersTrace runs the greedy Myers forward pass, snapshotting the frontier V at the start of each
// depth so the backtrack can reconstruct the path.
func myersTrace(a, b []string) (trace [][]int, endX, endY int) {
	n, m := len(a), len(b)
	max := n + m
	offset := max
	v := make([]int, 2*max+1)
	for d := 0; d <= max; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1] // move down: an insertion from b
			} else {
				x = v[offset+k-1] + 1 // move right: a deletion from a
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return trace, n, m
			}
		}
	}
	return trace, n, m
}

// myersBacktrack walks the trace from the end to the origin, emitting ops in forward order.
func myersBacktrack(a, b []string, trace [][]int, x, y int) []diffLine {
	max := len(a) + len(b)
	offset := max
	var ops []diffLine
	emit := func(op diffOp, text string) { ops = append(ops, diffLine{op, text}) }

	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY { // diagonal: equal lines
			emit(opEqual, a[x-1])
			x--
			y--
		}
		if d > 0 {
			if x == prevX {
				emit(opAdd, b[y-1]) // came from a down move
			} else {
				emit(opDel, a[x-1]) // came from a right move
			}
		}
		x, y = prevX, prevY
	}
	// ops were emitted from the end backward; reverse to forward order.
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

// wordRun is a span of a line for intra-line highlighting: changed marks it as inserted/deleted.
type wordRun struct {
	text    string
	changed bool
}

var wordTokenSplit = regexp.MustCompile(`\S+|\s+`)

// tokenizeWords splits s into alternating word / whitespace tokens, so joining them reconstructs s.
func tokenizeWords(s string) []string {
	return wordTokenSplit.FindAllString(s, -1)
}

// diffWords computes token-level runs for a changed line pair, so only the words that actually
// differ are highlighted (prose that changed a few words doesn't flash as a whole red/green block).
func diffWords(a, b string) (aRuns, bRuns []wordRun) {
	for _, op := range diffLines(tokenizeWords(a), tokenizeWords(b)) {
		switch op.op {
		case opEqual:
			aRuns = append(aRuns, wordRun{op.text, false})
			bRuns = append(bRuns, wordRun{op.text, false})
		case opDel:
			aRuns = append(aRuns, wordRun{op.text, true})
		case opAdd:
			bRuns = append(bRuns, wordRun{op.text, true})
		}
	}
	return aRuns, bRuns
}

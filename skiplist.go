package sortedset

import (
	"fmt"
	"math/rand"
)

const (
	maxLevel int = 32
	p            = 0.25
)

type level struct {
	forward *node
	span    int
}

type node struct {
	member   string
	score    float64
	backward *node
	levels   []level
}

func newNode(lvl int, score float64, member string) *node {
	var n *node
	switch lvl {
	case 1:
		b := new(struct {
			n node
			l [1]level
		})
		b.n.levels, n = b.l[:], &b.n
	case 2:
		b := new(struct {
			n node
			l [2]level
		})
		b.n.levels, n = b.l[:], &b.n
	case 3:
		b := new(struct {
			n node
			l [3]level
		})
		b.n.levels, n = b.l[:], &b.n
	case 4:
		b := new(struct {
			n node
			l [4]level
		})
		b.n.levels, n = b.l[:], &b.n
	default:
		n = &node{levels: make([]level, lvl)}
	}
	n.member, n.score = member, score
	return n
}

type skipList struct {
	header *node
	tail   *node
	length int
	level  int
}

func newSkipList() *skipList {
	return &skipList{header: newNode(maxLevel, 0, ""), level: 1}
}

func randomLevel() int {
	lvl := 1

	for lvl < maxLevel && rand.Float64() < p {
		lvl++
	}

	return lvl
}

func less(s1 float64, m1 string, s2 float64, m2 string) bool {
	return s1 < s2 || (s1 == s2 && m1 < m2)
}

func (sl *skipList) insert(score float64, member string) *node {
	var update [maxLevel]*node
	var rank [maxLevel]int

	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		if i < sl.level-1 {
			rank[i] = rank[i+1]
		}

		for x.levels[i].forward != nil && less(x.levels[i].forward.score, x.levels[i].forward.member, score, member) {
			rank[i] += x.levels[i].span
			x = x.levels[i].forward
		}

		update[i] = x
	}

	lvl := randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			rank[i] = 0
			update[i] = sl.header
			update[i].levels[i].span = sl.length
		}
		sl.level = lvl
	}

	x = newNode(lvl, score, member)
	for i := 0; i < lvl; i++ {
		x.levels[i].forward = update[i].levels[i].forward
		update[i].levels[i].forward = x

		x.levels[i].span = update[i].levels[i].span - (rank[0] - rank[i])
		update[i].levels[i].span = (rank[0] - rank[i]) + 1
	}

	for i := lvl; i < sl.level; i++ {
		update[i].levels[i].span++
	}

	if update[0] != sl.header {
		x.backward = update[0]
	}

	if x.levels[0].forward != nil {
		x.levels[0].forward.backward = x
	} else {
		sl.tail = x
	}

	sl.length++
	return x
}

func (sl *skipList) rank(score float64, member string) int {
	r := 0
	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		for x.levels[i].forward != nil && !less(score, member, x.levels[i].forward.score, x.levels[i].forward.member) {
			r += x.levels[i].span
			x = x.levels[i].forward
		}

		if x != sl.header && x.member == member {
			return r
		}
	}

	return 0
}

func (sl *skipList) delete(score float64, member string) bool {
	var update [maxLevel]*node

	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		for x.levels[i].forward != nil && less(x.levels[i].forward.score, x.levels[i].forward.member, score, member) {
			x = x.levels[i].forward
		}
		update[i] = x
	}

	x = x.levels[0].forward

	if x == nil || x.score != score || x.member != member {
		return false
	}

	for i := 0; i < sl.level; i++ {
		if update[i].levels[i].forward == x {
			update[i].levels[i].span += x.levels[i].span - 1
			update[i].levels[i].forward = x.levels[i].forward
		} else {
			update[i].levels[i].span--
		}
	}

	if x.levels[0].forward != nil {
		x.levels[0].forward.backward = x.backward
	} else {
		sl.tail = x.backward
	}

	for sl.level > 1 && sl.header.levels[sl.level-1].forward == nil {
		sl.level--
	}
	sl.length--
	return true
}

func (sl *skipList) byRank(r int) *node {
	traversed := 0
	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		for x.levels[i].forward != nil && traversed+x.levels[i].span <= r {
			traversed += x.levels[i].span
			x = x.levels[i].forward
		}

		if traversed == r {
			return x
		}
	}

	return nil
}

func (sl *skipList) firstAtLeast(min float64) *node {
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.levels[i].forward != nil && x.levels[i].forward.score < min {
			x = x.levels[i].forward
		}
	}

	return x.levels[0].forward
}

func (sl *skipList) dump() {
	for i := sl.level - 1; i >= 0; i-- {
		fmt.Printf("L%d: head", i+1)
		for x := sl.header; x.levels[i].forward != nil; x = x.levels[i].forward {
			f := x.levels[i].forward

			fmt.Printf(" - (%d) -> %s:%g", x.levels[i].span, f.member, f.score)
		}
		fmt.Println()
	}
}

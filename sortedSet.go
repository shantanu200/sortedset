package sortedset

import (
	"cmp"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
)

var nextSetID atomic.Uint64

type SortedSet struct {
	id   uint64
	mu   sync.RWMutex
	dict map[string]float64
	zsl  *skipList
}

func NewSortedSet() *SortedSet {
	return &SortedSet{
		id:   nextSetID.Add(1),
		dict: map[string]float64{},
		zsl:  newSkipList(),
	}
}

func (z *SortedSet) add(member string, score float64) bool {
	old, exists := z.dict[member]
	if exists {
		if old != score {
			z.zsl.delete(old, member)
			z.zsl.insert(score, member)
			z.dict[member] = score
		}

		return false
	}

	z.zsl.insert(score, member)
	z.dict[member] = score
	return true
}

func (z *SortedSet) Add(member string, score float64) bool {
	z.mu.Lock()
	defer z.mu.Unlock()

	return z.add(member, score)
}

func (z *SortedSet) Score(member string) (float64, bool) {
	z.mu.RLock()
	defer z.mu.RUnlock()

	s, ok := z.dict[member]
	return s, ok
}

func (z *SortedSet) Rem(member string) bool {
	z.mu.Lock()
	defer z.mu.Unlock()

	score, ok := z.dict[member]
	if !ok {
		return false
	}
	z.zsl.delete(score, member)
	delete(z.dict, member)
	return true
}

func (z *SortedSet) Rank(member string) int {
	z.mu.RLock()
	defer z.mu.RUnlock()

	score, ok := z.dict[member]
	if !ok {
		return -1
	}

	return z.zsl.rank(score, member) - 1
}

func (z *SortedSet) limit(start, stop int) (int, int) {
	n := z.zsl.length

	if start < 0 {
		start += n
	}

	if stop < 0 {
		stop += n
	}

	if start < 0 {
		start = 0
	}

	if stop >= n {
		stop = n - 1
	}

	return start, stop
}

func (z *SortedSet) Range(start, stop int) []string {
	z.mu.RLock()
	defer z.mu.RUnlock()
	start, stop = z.limit(start, stop)

	if start > stop {
		return nil
	}

	out := make([]string, 0, stop-start+1)
	x := z.zsl.byRank(start + 1)

	for i := start; i <= stop && x != nil; i++ {
		out = append(out, x.member)
		x = x.levels[0].forward
	}

	return out
}

func (z *SortedSet) RevRange(start, stop int) []string {
	z.mu.RLock()
	defer z.mu.RUnlock()

	start, stop = z.limit(start, stop)

	if start > stop {
		return nil
	}

	n := z.zsl.length

	out := make([]string, 0, stop-start+1)

	x := z.zsl.byRank(n - start)

	for i := start; i <= stop && x != nil; i++ {
		out = append(out, x.member)
		x = x.backward
	}

	return out
}

func (z *SortedSet) RangeByScore(min, max float64) []string {
	z.mu.RLock()
	defer z.mu.RUnlock()

	if min > max {
		return nil
	}

	out := []string{}

	first := z.zsl.firstAtLeast(min)

	for x := first; x != nil && x.score <= max; x = x.levels[0].forward {
		out = append(out, x.member)
	}

	return out
}

func (z *SortedSet) Incrby(member string, incr float64) (float64, bool) {
	z.mu.Lock()
	defer z.mu.Unlock()

	s, ok := z.dict[member]

	if !ok {
		return 0, false
	}

	nScore := s + incr
	z.add(member, nScore)

	return nScore, true
}

func (z *SortedSet) each(fn func(member string, score float64)) {
	for x := z.zsl.header.levels[0].forward; x != nil; x = x.levels[0].forward {
		fn(x.member, x.score)
	}
}

func (z *SortedSet) Card() int64 {
	z.mu.RLock()
	defer z.mu.RUnlock()

	return int64(z.zsl.length)
}

func (z *SortedSet) Count(min, max float64) int64 {
	z.mu.RLock()
	defer z.mu.RUnlock()

	if min > max {
		return 0
	}

	cnt := int64(0)

	first := z.zsl.firstAtLeast(min)

	for x := first; x != nil && x.score <= max; x = x.levels[0].forward {
		cnt += 1
	}

	return cnt
}

type Aggregate int

const (
	AggSum Aggregate = iota
	AggMin
	AggMax
)

func combine(agg Aggregate, a, b float64) float64 {
	switch agg {
	case AggMin:
		if b < a {
			return b
		}

		return a
	case AggMax:
		if b > a {
			return b
		}
		return a
	default:
		return a + b
	}
}

func Inter(sets []*SortedSet, weights []float64, agg Aggregate) *SortedSet {
	out := NewSortedSet()

	if len(sets) == 0 {
		return out
	}

	if weights == nil {
		weights = make([]float64, len(sets))

		for i := range weights {
			weights[i] = 1
		}
	}

	if len(weights) != len(sets) {
		return out
	}

	uniq := slices.Clone(sets)
	slices.SortFunc(uniq, func(a, b *SortedSet) int {
		return cmp.Compare(a.id, b.id)
	})

	uniq = slices.Compact(uniq)

	for _, m := range interMatches(sets, uniq, weights, agg) {
		out.add(m.member, m.score)
	}

	return out
}

type match struct {
	member string
	score  float64
}

func interMatches(sets, uniq []*SortedSet, weights []float64, agg Aggregate) []match {
	for _, s := range uniq {
		s.mu.RLock()
	}
	defer func() {
		for _, s := range uniq {
			s.mu.RUnlock()
		}
	}()

	order := make([]int, len(sets))

	for i := range order {
		order[i] = i
	}

	sort.Slice(order, func(a, b int) bool {
		return sets[order[a]].zsl.length < sets[order[b]].zsl.length
	})

	smallest := order[0]
	if sets[smallest].zsl.length == 0 {
		return nil
	}

	matches := make([]match, 0, sets[smallest].zsl.length)
	sets[smallest].each(func(member string, score float64) {
		total := score * weights[smallest]

		for _, j := range order[1:] {
			s, ok := sets[j].dict[member]
			if !ok {
				return
			}
			total = combine(agg, total, s*weights[j])
		}

		matches = append(matches, match{member, total})
	})

	return matches
}

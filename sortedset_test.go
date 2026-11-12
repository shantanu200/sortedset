package sortedset

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestIncrbyDeadlock(t *testing.T) {
	z := NewSortedSet()
	z.Add("a", 1)
	done := make(chan struct{})
	go func() { _, _ = z.Incrby("a", 1); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Incrby deadlocked")
	}
}

func TestCardCountRace(t *testing.T) {
	z := NewSortedSet()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				z.Add(fmt.Sprint(i, j), float64(j))
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = z.Card()
				_ = z.Count(0, 500)
			}
		}()
	}
	wg.Wait()
}

func TestInterConcurrent(t *testing.T) {
	a, b := NewSortedSet(), NewSortedSet()
	for i := 0; i < 200; i++ {
		a.Add(fmt.Sprint(i), float64(i))
		b.Add(fmt.Sprint(i), float64(i))
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				Inter([]*SortedSet{a, b}, nil, AggSum)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				Inter([]*SortedSet{b, a, a}, nil, AggMax)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				a.Add(fmt.Sprint(j), float64(j*2))
				b.Rem(fmt.Sprint(j))
			}
		}()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Inter deadlocked")
	}
}

func TestSkipListInvariants(t *testing.T) {
	z := NewSortedSet()
	for i := 0; i < 20000; i++ {
		k := fmt.Sprint(i % 3000)
		switch i % 5 {
		case 0, 1, 2:
			z.Add(k, float64((i*7919)%1000))
		case 3:
			z.Rem(k)
		case 4:
			z.Incrby(k, 1)
		}
	}
	all := z.Range(0, -1)
	if len(all) != len(z.dict) || int(z.Card()) != len(z.dict) {
		t.Fatalf("len mismatch: range=%d dict=%d", len(all), len(z.dict))
	}
	for i, m := range all {
		if r := z.Rank(m); r != i {
			t.Fatalf("rank(%s)=%d want %d", m, r, i)
		}
		if i > 0 {
			p := all[i-1]
			if !less(z.dict[p], p, z.dict[m], m) {
				t.Fatalf("order broken at %d", i)
			}
		}
	}
	rev := z.RevRange(0, -1)
	for i := range rev {
		if rev[i] != all[len(all)-1-i] {
			t.Fatalf("revrange mismatch at %d", i)
		}
	}
	if got := z.Range(10, 19); len(got) != 10 || got[0] != all[10] {
		t.Fatalf("Range(10,19) wrong: %v", got)
	}
	if c := z.Count(100, 200); int(c) != len(z.RangeByScore(100, 200)) {
		t.Fatalf("Count != len(RangeByScore)")
	}
}

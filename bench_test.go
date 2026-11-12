package sortedset

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var sizes = []int{1_000, 100_000}

func benchKeys(prefix string, n int) []string {
	k := make([]string, n)
	for i := range k {
		k[i] = prefix + strconv.Itoa(i)
	}
	return k
}

func filled(n int) (*SortedSet, []string) {
	ks := benchKeys("member:", n)
	z := NewSortedSet()
	r := rand.New(rand.NewPCG(1, 2))
	for _, k := range ks {
		z.Add(k, float64(r.IntN(n*10)))
	}
	return z, ks
}

func interPair(n int) (*SortedSet, *SortedSet, []string) {
	a, b := NewSortedSet(), NewSortedSet()
	ks := benchKeys("m", n)
	for i := 0; i < n; i++ {
		a.Add(ks[i], float64(i))
		b.Add("m"+strconv.Itoa(i+n/2), float64(i))
	}
	return a, b, ks
}

func BenchmarkOps(b *testing.B) {
	for _, n := range sizes {
		run := func(name string, fn func(z *SortedSet, ks []string, r *rand.Rand)) {
			b.Run(fmt.Sprintf("n=%d/op=%s", n, name), func(b *testing.B) {
				z, ks := filled(n)
				r := rand.New(rand.NewPCG(3, 4))
				b.ReportAllocs()
				for b.Loop() {
					fn(z, ks, r)
				}
			})
		}

		run("Score", func(z *SortedSet, ks []string, r *rand.Rand) { z.Score(ks[r.IntN(n)]) })
		run("Rank", func(z *SortedSet, ks []string, r *rand.Rand) { z.Rank(ks[r.IntN(n)]) })
		run("Add-update", func(z *SortedSet, ks []string, r *rand.Rand) {
			z.Add(ks[r.IntN(n)], float64(r.IntN(n*10)))
		})
		extra := benchKeys("extra:", n)
		run("Add+Rem-new", func(z *SortedSet, ks []string, r *rand.Rand) {
			k := extra[r.IntN(n)]
			z.Add(k, float64(r.IntN(n*10)))
			z.Rem(k)
		})
		run("Range-top10", func(z *SortedSet, ks []string, r *rand.Rand) { z.Range(0, 9) })
		run("Range-mid10", func(z *SortedSet, ks []string, r *rand.Rand) { z.Range(n/2, n/2+9) })
		run("RevRange-top10", func(z *SortedSet, ks []string, r *rand.Rand) { z.RevRange(0, 9) })
		run("RangeByScore-10", func(z *SortedSet, ks []string, r *rand.Rand) {
			lo := float64(r.IntN(n*10 - 100))
			z.RangeByScore(lo, lo+99)
		})
		run("Count-10", func(z *SortedSet, ks []string, r *rand.Rand) {
			lo := float64(r.IntN(n*10 - 100))
			z.Count(lo, lo+99)
		})
	}
}

func BenchmarkInter(b *testing.B) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			a, b2, _ := interPair(n)
			b.ReportAllocs()
			for b.Loop() {
				Inter([]*SortedSet{a, b2}, nil, AggSum)
			}
		})
	}
}

func BenchmarkInterWriterStall(b *testing.B) {
	const n = 100_000
	a, b2, ks := interPair(n)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			Inter([]*SortedSet{a, b2}, nil, AggSum)
		}
	}()
	r := rand.New(rand.NewPCG(7, 8))
	lat := make([]time.Duration, 0, 1<<14)
	for b.Loop() {
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			i := r.IntN(n)
			t0 := time.Now()
			a.Add(ks[i], float64(i))
			lat = append(lat, time.Since(t0))
			time.Sleep(200 * time.Microsecond)
		}
	}
	close(stop)
	wg.Wait()
	slices.Sort(lat)
	stalled := 0
	for _, d := range lat {
		if d > time.Millisecond {
			stalled++
		}
	}
	b.ReportMetric(float64(lat[len(lat)*50/100].Nanoseconds()), "p50-ns")
	b.ReportMetric(float64(lat[len(lat)*99/100].Nanoseconds()), "p99-ns")
	b.ReportMetric(float64(lat[len(lat)-1].Nanoseconds()), "max-ns")
	b.ReportMetric(100*float64(stalled)/float64(len(lat)), "%stalled>1ms")
}

func BenchmarkParallel(b *testing.B) {
	const n = 100_000
	cases := []struct {
		name    string
		readPct int
		read    func(z *SortedSet, k string)
	}{
		{"Score/read=100", 100, func(z *SortedSet, k string) { z.Score(k) }},
		{"Rank/read=100", 100, func(z *SortedSet, k string) { z.Rank(k) }},
		{"Rank/read=90", 90, func(z *SortedSet, k string) { z.Rank(k) }},
		{"Add/read=0", 0, nil},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			z, ks := filled(n)
			var seed atomic.Uint64
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				s := seed.Add(1)
				r := rand.New(rand.NewPCG(s, s*7919))
				for pb.Next() {
					k := ks[r.IntN(n)]
					if r.IntN(100) < c.readPct {
						c.read(z, k)
					} else {
						z.Add(k, float64(r.IntN(n*10)))
					}
				}
			})
		})
	}
}

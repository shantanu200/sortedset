# sortedset

A thread-safe sorted set for Go, modeled on Redis `ZSET`. Members are strings, each with a `float64` score. The set keeps them ordered by score, with ties broken by member name.

It uses the same layout as Redis: a hash map from member to score for O(1) lookups, plus a skip list with span counts, so rank lookups and range-by-rank are O(log n) instead of a linear walk.

## Install

```bash
go get github.com/shantanu200/sortedset
```

Requires the Go version in `go.mod` (1.27.3).

## Usage

```go
package main

import (
	"fmt"

	"github.com/shantanu200/sortedset"
)

func main() {
	z := sortedset.NewSortedSet()
	z.Add("alice", 10)
	z.Add("bob", 20)
	z.Add("carol", 30)

	fmt.Println(z.Range(0, -1))
	fmt.Println(z.RevRange(0, 0))
	fmt.Println(z.Rank("bob"))
	fmt.Println(z.RangeByScore(15, 30))
	fmt.Println(z.Count(15, 30))

	score, _ := z.Incrby("alice", 25)
	fmt.Println(score, z.Range(0, -1))

	other := sortedset.NewSortedSet()
	other.Add("bob", 1)
	other.Add("alice", 1)

	both := sortedset.Inter([]*sortedset.SortedSet{z, other}, nil, sortedset.AggSum)
	fmt.Println(both.Range(0, -1))
}
```

Output:

```
[alice bob carol]
[carol]
1
[bob carol]
2
35 [bob carol alice]
[bob alice]
```

## API

| Method | Returns | Notes |
|---|---|---|
| `NewSortedSet()` | `*SortedSet` | |
| `Add(member, score)` | `bool` | `true` if the member is new. Updates the score if it already exists. |
| `Rem(member)` | `bool` | `true` if the member was removed. |
| `Score(member)` | `float64, bool` | |
| `Incrby(member, incr)` | `float64, bool` | Returns the new score. If the member does not exist it returns `false` and changes nothing (Redis would create it). |
| `Rank(member)` | `int` | 0-based rank in ascending order, `-1` if missing. |
| `Card()` | `int64` | Number of members. |
| `Range(start, stop)` | `[]string` | By rank, ascending, both ends inclusive. Negative indexes count from the end, so `Range(0, -1)` returns everything. |
| `RevRange(start, stop)` | `[]string` | Same, descending. |
| `RangeByScore(min, max)` | `[]string` | Members with `min <= score <= max`. |
| `Count(min, max)` | `int64` | Number of members with `min <= score <= max`. |
| `Inter(sets, weights, agg)` | `*SortedSet` | New set of members present in every input. Each score is multiplied by its set's weight, then combined with `AggSum`, `AggMin` or `AggMax`. `nil` weights means all 1. If `len(weights) != len(sets)` the result is empty. |

## Complexity

| Operation | Cost |
|---|---|
| `Score`, `Card` | O(1) |
| `Add`, `Rem`, `Incrby`, `Rank` | O(log n) expected |
| `Range`, `RevRange` | O(log n + k) for k results |
| `RangeByScore`, `Count` | O(log n + k) for k matches |
| `Inter` | one map lookup per member of the smallest set per other set, plus O(r log r) to build r results |

## Thread safety

Every set has its own `sync.RWMutex`. Reads (`Score`, `Rank`, `Range`, `RevRange`, `RangeByScore`, `Count`, `Card`) take the read lock and writes take the write lock, so every method is safe to call from many goroutines.

`Inter` works on several sets at once. It read-locks each distinct input exactly once, in a fixed order based on a per-set ID. Two concurrent `Inter` calls that list the same sets in different orders cannot deadlock, and passing the same set twice is fine. It collects the matches while holding the locks and builds the result after releasing them, so writers on the inputs are blocked only during the lookups.

Returned slices are copies and stay valid after the set changes.

## Benchmarks

Measured on an Intel i7-10750H laptop (6 cores, 12 threads) with 100k members. Figures are `benchstat` medians of 8 runs pinned to 4 CPUs. Laptop CPU frequency scaling adds noise, so read them as rough numbers.

| Operation | Time | Allocs |
|---|---|---|
| `Score` | ~70 ns | 0 |
| `Rank` | 856 ns | 0 |
| `Add` (update existing) | 1.92 µs | 1 |
| `Add` + `Rem` (new member) | 1.37 µs | 2 |
| `Range` top 10 | 135 ns | 1 |
| `Range` middle 10 | 194 ns | 1 |
| `RevRange` top 10 | 190 ns | 1 |
| `RangeByScore` (~10 hits) | 1.14 µs | 2 |
| `Count` (~10 hits) | 815 ns | 0 |
| `Inter` (2 × 100k, 50% overlap) | 20.1 ms | 50k |

With many goroutines sharing one set:

- Read-only `Rank` scales with cores: 917 ns/op on 1 core, 109 ns/op on 12 threads (wall time divided by total operations).
- With 10% writes, adding cores doesn't help (about 1.2 µs/op at any core count). There is one lock per set, so writers run one at a time, and Go's `RWMutex` holds back new readers while a writer is waiting.
- An `Add` issued while another goroutine runs `Inter` on the same 100k set waits 664 ns at the median and 6.1 ms at p99. The p99 is the lookup phase of `Inter`, which still holds the read locks.

To reproduce:

```bash
go test -race ./...
go test -run '^$' -bench '^Benchmark(Ops|Inter)$' -benchmem -count=10 ./...
go test -run '^$' -bench 'InterWriterStall' -benchtime=1x -count=6 ./...
go test -run '^$' -bench 'Parallel' -cpu=1,4,12 -count=8 ./...
```

Run `BenchmarkInterWriterStall` with `-benchtime=1x`; each iteration is a fixed 1.5 s sampling window. Compare runs with [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

## Limitations

- Write-heavy workloads don't scale past one core. Sharding the set would break global ordering, so fixing this needs a different design, such as a single writer goroutine applying batched updates.
- `NaN` scores are not rejected and break the ordering. `+Inf` and `-Inf` are fine, but `Incrby` can produce `NaN` from `+Inf` plus `-Inf`.
- `Count` walks every match, so `Count(-inf, +inf)` is O(n). Using the span counts would make it O(log n).
- No union, range removal or persistence yet.

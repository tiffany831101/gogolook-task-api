package repository

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
)

// benchStoreSize is the number of tasks preloaded before each benchmark.
const benchStoreSize = 10_000

func newSeededRepo(b *testing.B, n int) *MemoryTaskRepository {
	b.Helper()
	repo := NewMemoryTaskRepository()
	for i := 0; i < n; i++ {
		if _, err := repo.Create(context.Background(), model.Task{Name: "task"}); err != nil {
			b.Fatal(err)
		}
	}
	return repo
}

// BenchmarkMemoryTaskRepository_List measures concurrent readers. Because List
// only takes a read lock, throughput should scale with GOMAXPROCS, and the
// cost should depend on the page size rather than the store size.
func BenchmarkMemoryTaskRepository_List(b *testing.B) {
	for _, limit := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("limit=%d", limit), func(b *testing.B) {
			repo := newSeededRepo(b, benchStoreSize)
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, _, err := repo.List(ctx, ListParams{Limit: limit}); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}

// BenchmarkMemoryTaskRepository_ListStoreSize shows that a page costs the
// same whether the store holds 1k or 100k tasks.
func BenchmarkMemoryTaskRepository_ListStoreSize(b *testing.B) {
	for _, size := range []int{1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("tasks=%d", size), func(b *testing.B) {
			repo := newSeededRepo(b, size)
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := repo.List(ctx, ListParams{Offset: size / 2, Limit: 100}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMemoryTaskRepository_Create measures concurrent writers, which are
// serialised by the write lock.
func BenchmarkMemoryTaskRepository_Create(b *testing.B) {
	repo := NewMemoryTaskRepository()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := repo.Create(ctx, model.Task{Name: "task"}); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// BenchmarkMemoryTaskRepository_Mixed runs concurrent reads (List) and writes
// (Update) at different ratios. Comparing the results shows how much write
// contention reduces the benefit of the RWMutex for read-heavy traffic.
func BenchmarkMemoryTaskRepository_Mixed(b *testing.B) {
	for _, readPercent := range []int{100, 90, 50, 0} {
		b.Run(fmt.Sprintf("read=%d%%", readPercent), func(b *testing.B) {
			repo := newSeededRepo(b, benchStoreSize)
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if rand.IntN(100) < readPercent {
						if _, _, err := repo.List(ctx, ListParams{Limit: 100}); err != nil {
							b.Error(err)
							return
						}
						continue
					}
					id := int64(rand.IntN(benchStoreSize)) + 1
					if _, err := repo.Update(ctx, model.Task{ID: id, Name: "updated", Status: model.StatusCompleted}); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}

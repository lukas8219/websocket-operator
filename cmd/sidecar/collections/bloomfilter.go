package collections

import (
	boom "github.com/tylertreat/BoomFilters"
)

type BloomFilter struct {
	impl *boom.CountingBloomFilter
}

func New(size uint) *BloomFilter {
	return &BloomFilter{
		impl: boom.NewDefaultCountingBloomFilter(size, 0.01),
	}
}

func (b *BloomFilter) Add(item string) *BloomFilter {
	b.impl.Add([]byte(item))
	return b
}

func (b *BloomFilter) Contains(item string) bool {
	return b.impl.Test([]byte(item))
}

func (b *BloomFilter) Remove(item string) bool {
	return b.impl.TestAndRemove([]byte(item))
}

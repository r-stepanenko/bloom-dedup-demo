package bloom

import (
	"bloom-dedup-demo/internal/bitset"
	"math"
)

type Filter struct {
	bits *bitset.BitSet
	m    uint64
	k    int
	hash string
}

type CountingFilter struct {
	counters []uint16
	m        uint64
	k        int
	hash     string
}

// Создание нового фильтра Блума
func NewFilter(expectedItems int, p float64, hashFamily string) (*Filter, error) {
	m, k, err := Params(expectedItems, p)
	if err != nil {
		return nil, err
	}
	bs, err := bitset.New(uint(m))
	if err != nil {
		return nil, err
	}
	return &Filter{bs, uint64(m), k, hashFamily}, nil
}

// Создание нового фильтра Блума-счётчика
func NewCountingFilter(expectedItems int, p float64, hashFamily string) (*CountingFilter, error) {
	m, k, err := Params(expectedItems, p)
	if err != nil {
		return nil, err
	}
	return &CountingFilter{make([]uint16, m), uint64(m), k, hashFamily}, nil
}

// Количество байтов для фильтра Блума
func (f *Filter) MemoryBytes() int {
	return int((f.m + 7) / 8)
}

// Размер массива счётчиков в байтах
func (f *CountingFilter) MemoryBytes() int {
	return len(f.counters) * 2
}

// Получение индексов в соответстии с хешем
func (f *Filter) indexes(key string) []uint64 {
	if f.hash == "fnv64_double_hashing" {
		return getIndexes(key, f.k, f.m)
	}
	return getIndexesSHA256(key, f.k, f.m)
}

// Получение индексов в соответстии с хешем
func (f *CountingFilter) indexes(key string) []uint64 {
	if f.hash == "fnv64_double_hashing" {
		return getIndexes(key, f.k, f.m)
	}
	return getIndexesSHA256(key, f.k, f.m)
}

// Установка битов в массиве
func (f *Filter) Add(key string) {
	for _, idx := range f.indexes(key) {
		f.bits.Set(uint(idx))
	}
}

// Установка счётчиков
func (f *CountingFilter) Add(key string) {
	for _, idx := range f.indexes(key) {
		if f.counters[idx] < math.MaxUint16 {
			f.counters[idx]++
		}
	}
}

// Проверка на дубль
func (f *Filter) MayContain(key string) bool {
	for _, idx := range f.indexes(key) {
		if !f.bits.Get(uint(idx)) {
			return false
		}
	}
	return true
}

// Проверка на дубль
func (f *CountingFilter) MayContain(key string) bool {
	for _, idx := range f.indexes(key) {
		if f.counters[idx] == 0 {
			return false
		}
	}
	return true
}

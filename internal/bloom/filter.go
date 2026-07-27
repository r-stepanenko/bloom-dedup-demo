package bloom

import (
	"bloom-dedup-demo/internal/model"
	"crypto/sha256"
	"encoding/binary"
	"hash/fnv"
	"sort"
	"time"
)

// Точная дедупликация событий через map
// Возвращает events, unique, duplicates, durationMs, memoryBytes, error
func MapFilter(events []model.Event) ([]model.Event, int, int, int64, int, error) {
	start := time.Now()
	total := len(events)
	eventsMap := make(map[string]model.Event, total)
	result := make([]model.Event, 0, total)
	for i, event := range events {
		eventsMap[event.EventHash] = events[i]
	}
	unique := len(eventsMap)
	duplicates := total - unique
	duration := time.Since(start).Milliseconds()
	memory := estimateMapMemory(eventsMap)
	for _, event := range eventsMap {
		result = append(result, event)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Seq < result[j].Seq
	})
	return result, unique, duplicates, duration, memory, nil
}

// Рассчёт оценки map в байтах (примерное значение)
func estimateMapMemory(m map[string]model.Event) int {
	const mapBucketOverhead = 48
	total := 0
	for k, v := range m {
		total += len(k)
		total += len(v.EventID) + len(v.Source) + len(v.Timestamp)
		total += 8
		total += mapBucketOverhead
	}
	return total
}

// Вычисляет два независимых хеша строки для дальнейшего использования в схеме двойного хеширования
// Возвращает h1, h2 - базовые хеш-значения
func Hash(str string) (uint64, uint64) {
	h1 := fnv.New64()
	message := []byte(str)
	h1.Write(message)
	h1Value := h1.Sum64()
	h2 := fnv.New64a()
	message2 := []byte(str)
	h2.Write(message2)
	h2Value := h2.Sum64()
	return h1Value, h2Value
}

// Вычисляет k позиций в битовом массиве размера m для ключа key методом двойного хеширования
// Возвращает массив из k индексов битового массива
func getIndexes(key string, k int, m uint64) []uint64 {
	h1, h2 := Hash(key)
	indexes := make([]uint64, k)
	for i := 0; i < k; i++ {
		indexes[i] = (h1 + uint64(i)*h2) % m
	}
	return indexes
}

func getIndexesSHA256(key string, k int, m uint64) []uint64 {
	sum := sha256.Sum256([]byte(key))
	indexes := make([]uint64, k)
	for i := 0; i < k; i++ {
		offset := (i * 4) % 29
		v := binary.BigEndian.Uint32(sum[offset : offset+4])
		indexes[i] = uint64(v) % m
	}
	return indexes
}

// Фильтр Блума
// Возвращает unique, duplicates, durationMs, memoryBytes, error
func BloomFilter(events []model.Event, expectedItems int, hash string, p float64) (int, int, int64, int, error) {
	start := time.Now()
	f, err := NewFilter(expectedItems, p, hash)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	duplicates := 0
	for _, event := range events {
		if f.MayContain(event.EventHash) {
			duplicates++
		} else {
			f.Add(event.EventHash)
		}
	}
	unique := len(events) - duplicates
	duration := time.Since(start).Milliseconds()
	return unique, duplicates, duration, f.MemoryBytes(), nil
}

// Фильтр Блума счётчик
// Возвращает unique, duplicates, durationMs, memoryBytes, error
func CountingBloomFilter(events []model.Event, expectedItems int, hash string, p float64) (int, int, int64, int, error) {
	start := time.Now()
	f, err := NewCountingFilter(expectedItems, p, hash)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	duplicates := 0
	for _, event := range events {
		if f.MayContain(event.EventHash) {
			duplicates++
		}
		f.Add(event.EventHash)
	}
	unique := len(events) - duplicates
	duration := time.Since(start).Milliseconds()
	return unique, duplicates, duration, f.MemoryBytes(), nil
}

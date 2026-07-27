package bloom

import (
	"fmt"
	"testing"
)

func TestFilterAddMayContain(t *testing.T) {
	f, err := NewFilter(1000, 0.01, "fnv64_double_hashing")
	if err != nil {
		t.Fatalf("NewFilter вернул ошибку: %v", err)
	}
	if f.MayContain("tratata") {
		t.Errorf("пустой фильтр не должен содержать ключ")
	}
	f.Add("tratata")
	if !f.MayContain("tratata") {
		t.Errorf("фильтр должен вернуть true для ключа tratata")
	}
	if f.MemoryBytes() <= 0 {
		t.Errorf("ожидали положительный MemoryBytes")
	}
}

func TestFilterSHA256(t *testing.T) {
	f, err := NewFilter(1000, 0.01, "sha256_slices")
	if err != nil {
		t.Fatalf("NewFilter вернул ошибку: %v", err)
	}
	f.Add("hahahohohehe")
	if !f.MayContain("hahahohohehe") {
		t.Errorf("фильтр должен вернуть true для ключа hahahohohehe")
	}
}

func TestFilter1(t *testing.T) {
	f, err := NewFilter(10000, 0.01, "fnv64_double_hashing")
	if err != nil {
		t.Fatalf("NewFilter вернул ошибку: %v", err)
	}
	keys := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		keys = append(keys, fmt.Sprintf("%x", i))
	}
	for _, k := range keys {
		f.Add(k)
	}
	for _, k := range keys {
		if !f.MayContain(k) {
			t.Fatalf("фильтр Блума всегда показывает, что элемента нет, ключ %s", k)
		}
	}
}

func TestFilterInvalidParams(t *testing.T) {
	_, err := NewFilter(0, 0.01, "fnv64_double_hashing")
	if err == nil {
		t.Errorf("ожидали ошибку при expected_items=0")
	}
	_, err = NewFilter(1000, 0, "fnv64_double_hashing")
	if err == nil {
		t.Errorf("ожидали ошибку при p=0")
	}
}

func TestCountingFilterAdd(t *testing.T) {
	f, err := NewCountingFilter(1000, 0.01, "fnv64_double_hashing")
	if err != nil {
		t.Fatalf("NewCountingFilter вернул ошибку: %v", err)
	}
	key := "bebebe"
	if f.MayContain(key) {
		t.Errorf("пустой фильтр не должен содержать ключ")
	}
	f.Add(key)
	if !f.MayContain(key) {
		t.Errorf("после Add фильтр обязан вернуть true, ключ bebebe")
	}
	if f.MemoryBytes() <= 0 {
		t.Errorf("ожидали положительный MemoryBytes")
	}
}

func BenchmarkFilterAdd(b *testing.B) {
	f, err := NewFilter(b.N+1, 0.01, "fnv64_double_hashing")
	if err != nil {
		b.Fatal(err)
	}
	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("%x", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Add(keys[i])
	}
}

func BenchmarkFilterMayContain(b *testing.B) {
	f, err := NewFilter(100000, 0.01, "fnv64_double_hashing")
	if err != nil {
		b.Fatal(err)
	}
	keys := make([]string, 100000)
	for i := range keys {
		keys[i] = fmt.Sprintf("%x", i)
		f.Add(keys[i])
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.MayContain(keys[i%len(keys)])
	}
}

func BenchmarkCountingFilterAdd(b *testing.B) {
	f, err := NewCountingFilter(b.N+1, 0.01, "fnv64_double_hashing")
	if err != nil {
		b.Fatal(err)
	}
	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("%x", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Add(keys[i])
	}
}

func BenchmarkCountingFilterMayContain(b *testing.B) {
	f, err := NewCountingFilter(100000, 0.01, "fnv64_double_hashing")
	if err != nil {
		b.Fatal(err)
	}
	keys := make([]string, 100000)
	for i := range keys {
		keys[i] = fmt.Sprintf("%x", i)
		f.Add(keys[i])
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.MayContain(keys[i%len(keys)])
	}
}

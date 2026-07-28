package report

import (
	"bloom-dedup-demo/internal/bloom"
	"bloom-dedup-demo/internal/model"
	"fmt"
)

var DefaultRates = []float64{0.1, 0.05, 0.01, 0.001}

type FPRow struct {
	FalsePositiveRate       float64  `json:"false_positive_rate"`
	MBits                   int      `json:"m_bits"`
	KHashes                 int      `json:"k_hashes"`
	MemoryBytes             int      `json:"memory_bytes"`
	BloomMayDuplicate       *int     `json:"bloom_may_duplicate"`
	EstimatedFalsePositives *int     `json:"estimated_false_positives"`
	RealFalsePositiveRate   *float64 `json:"real_false_positive_rate"`
}

// Расчёт параметров
func ParamFPRows(expectedItems int, rates []float64) ([]FPRow, error) {
	if len(rates) == 0 {
		return nil, fmt.Errorf("список false_positive_rate не может быть пустым")
	}
	rows := make([]FPRow, 0, len(rates))
	for _, p := range rates {
		m, k, err := bloom.Params(expectedItems, p)
		if err != nil {
			return nil, fmt.Errorf("p=%v, %w", p, err)
		}
		rows = append(rows, FPRow{
			FalsePositiveRate: p,
			MBits:             m,
			KHashes:           k,
			MemoryBytes:       (m + 7) / 8,
		})
	}
	return rows, nil
}

// Расчёт необязательных параметров
func ExtraParamsFPRows(events []model.Event, expectedItems int, hashFamily string, rates []float64) ([]FPRow, error) {
	rows, err := ParamFPRows(expectedItems, rates)
	if err != nil {
		return nil, err
	}
	_, exactUnique, exactDup, _, _, err := bloom.MapFilter(events)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		_, bloomDup, _, _, err := bloom.BloomFilter(events, expectedItems, hashFamily, rows[i].FalsePositiveRate)
		if err != nil {
			return nil, fmt.Errorf("p=%v, %w", rows[i].FalsePositiveRate, err)
		}
		estFP := bloomDup - exactDup
		if estFP < 0 {
			estFP = 0
		}
		fpRate := 0.0
		if exactUnique > 0 {
			fpRate = float64(estFP) / float64(exactUnique)
		}
		rows[i].BloomMayDuplicate = &bloomDup
		rows[i].EstimatedFalsePositives = &estFP
		rows[i].RealFalsePositiveRate = &fpRate
	}
	return rows, nil
}

// Команда CLI талицы
func BuildFPTable(events []model.Event, expectedItems int, hash string, rates []float64) error {
	if len(rates) == 0 {
		return fmt.Errorf("список false_positive_rate не может быть пустым")
	}
	if hash != "f64" && hash != "s256" {
		return fmt.Errorf("hash должен быть f64 или s256")
	}
	if hash == "f64" {
		hash = "fnv64_double_hashing"
	} else {
		hash = "sha256_slices"
	}
	_, exactUnique, exactDup, _, _, err := bloom.MapFilter(events)
	if err != nil {
		return err
	}
	rows, err := ExtraParamsFPRows(events, expectedItems, hash, rates)
	if err != nil {
		return err
	}

	fmt.Printf("Сравнение значений false_positive_rate\n\n")
	fmt.Printf("Записей: %d, уникальных (map): %d, дублей (map): %d, expected_items: %d\n", len(events), exactUnique, exactDup, expectedItems)
	fmt.Printf("%-5s %-12s %-5s %-15s %-15s %-18s %-18s\n", "p", "m (бит)", "k", "Память (байт)", "Дубли (Блум)", "Ложные срабатывания", "Реальный FP rate")
	fmt.Printf("|-----|-------|-----|----------|---------|----------|----------|\n")
	for _, row := range rows {
		fmt.Printf("| %g | %d | %d | %d | %d | %d | %.6f |\n",
			row.FalsePositiveRate, row.MBits, row.KHashes, row.MemoryBytes,
			*row.BloomMayDuplicate, *row.EstimatedFalsePositives, *row.RealFalsePositiveRate)
	}
	return nil
}

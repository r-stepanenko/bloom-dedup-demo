package report

import (
	"bloom-dedup-demo/internal/bloom"
	"bloom-dedup-demo/internal/model"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SourceStats struct {
	TotalRecords            int  `json:"total_records"`
	ExactUnique             *int `json:"exact_unique"`
	ExactDuplicates         *int `json:"exact_duplicates"`
	BloomMayDuplicate       int  `json:"bloom_may_duplicate"`
	EstimatedFalsePositives *int `json:"estimated_false_positives"`
}

type Report struct {
	TotalRecords            int                    `json:"total_records"`
	BadLines                int                    `json:"bad_lines"`
	ExactUnique             *int                   `json:"exact_unique"`
	ExactDuplicates         *int                   `json:"exact_duplicates"`
	BloomNew                int                    `json:"bloom_new"`
	BloomMayDuplicate       int                    `json:"bloom_may_duplicate"`
	EstimatedFalsePositives *int                   `json:"estimated_false_positives"`
	RealFalsePositiveRate   *float64               `json:"real_false_positive_rate"`
	BloomMemoryBytes        int                    `json:"bloom_memory_bytes"`
	ExactMapMemoryBytes     *int                   `json:"exact_map_memory_bytes"`
	DurationMs              int64                  `json:"duration_ms"`
	MapDurationMs           *int64                 `json:"map_duration_ms"`
	BloomDurationMs         int64                  `json:"bloom_duration_ms"`
	FPTable                 []FPRow                `json:"fp_table"`
	BySource                map[string]SourceStats `json:"by_source"`
	InvalidSources          []string               `json:"invalid_sources"`
}

// Создание отчёта
// Входные данные: события, плохие строки, плохие источники, файл с конфигурацией, флаг пропуска
func BuildReport(events []model.Event, badLines []int, badSources []string, pathcfg string, mapMode bool) (*Report, error) {
	cfg, err := model.ReadConfig(pathcfg)
	if err != nil {
		return nil, err
	}
	total := len(events)

	var bloomNew, bloomDup, bloomMemory int
	var bloomDuration int64
	var err3 error
	if cfg.Mode == "bloom" {
		bloomNew, bloomDup, bloomDuration, bloomMemory, err3 = bloom.BloomFilter(events, cfg.ExpectedItems, cfg.HashFamily, cfg.FalsePositiveRate)
	} else {
		bloomNew, bloomDup, bloomDuration, bloomMemory, err3 = bloom.CountingBloomFilter(events, cfg.ExpectedItems, cfg.HashFamily, cfg.FalsePositiveRate)
	}
	if err3 != nil {
		return nil, err3
	}
	invalid := uniqueStrings(badSources)

	bS, err4 := BuildBySource(events, cfg.HashFamily, cfg.Mode, mapMode, cfg.FalsePositiveRate)
	if err4 != nil {
		return nil, err4
	}

	if mapMode {
		_, exactUnique, exactDup, mapDuration, mapMemory, err1 := bloom.MapFilter(events)
		if err1 != nil {
			return nil, err1
		}
		estFP := bloomDup - exactDup
		if estFP < 0 {
			estFP = 0
		}
		var fpRate float64
		if exactUnique > 0 {
			fpRate = float64(estFP) / float64(exactUnique)
		}

		fpTable, err5 := ExtraParamsFPRows(events, cfg.ExpectedItems, cfg.HashFamily, DefaultRates)
		if err5 != nil {
			return nil, err5
		}
		return &Report{
			TotalRecords:            total,
			BadLines:                len(badLines),
			ExactUnique:             &exactUnique,
			ExactDuplicates:         &exactDup,
			BloomNew:                bloomNew,
			BloomMayDuplicate:       bloomDup,
			EstimatedFalsePositives: &estFP,
			RealFalsePositiveRate:   &fpRate,
			BloomMemoryBytes:        bloomMemory,
			ExactMapMemoryBytes:     &mapMemory,
			DurationMs:              mapDuration + bloomDuration,
			MapDurationMs:           &mapDuration,
			BloomDurationMs:         bloomDuration,
			FPTable:                 fpTable,
			BySource:                bS,
			InvalidSources:          invalid,
		}, nil
	}
	fpTable, err5 := ParamFPRows(cfg.ExpectedItems, DefaultRates)
	if err5 != nil {
		return nil, err5
	}
	return &Report{
		TotalRecords:            total,
		BadLines:                len(badLines),
		ExactUnique:             nil,
		ExactDuplicates:         nil,
		BloomNew:                bloomNew,
		BloomMayDuplicate:       bloomDup,
		EstimatedFalsePositives: nil,
		RealFalsePositiveRate:   nil,
		BloomMemoryBytes:        bloomMemory,
		ExactMapMemoryBytes:     nil,
		MapDurationMs:           nil,
		BloomDurationMs:         bloomDuration,
		DurationMs:              bloomDuration,
		FPTable:                 fpTable,
		BySource:                bS,
		InvalidSources:          invalid,
	}, nil
}

// Потоковое создание отчёта без точного map
func BuildReportStreaming(path string, fls bool, pathcfg string) (*Report, error) {
	cfg, err := model.ReadConfig(pathcfg)
	if err != nil {
		return nil, err
	}

	var bf *bloom.Filter
	var cf *bloom.CountingFilter
	if cfg.Mode == "bloom" {
		bf, err = bloom.NewFilter(cfg.ExpectedItems, cfg.FalsePositiveRate, cfg.HashFamily)
	} else {
		cf, err = bloom.NewCountingFilter(cfg.ExpectedItems, cfg.FalsePositiveRate, cfg.HashFamily)
	}
	if err != nil {
		return nil, err
	}

	start := time.Now()
	total := 0
	bloomDup := 0
	bySource := make(map[string]SourceStats)

	badLines, badSources, err := model.StreamEvents(path, fls, func(e model.Event) {
		total++

		var mayDup bool
		if bf != nil {
			mayDup = bf.MayContain(e.EventHash)
			if !mayDup {
				bf.Add(e.EventHash)
			}
		} else {
			mayDup = cf.MayContain(e.EventHash)
			cf.Add(e.EventHash)
		}
		if mayDup {
			bloomDup++
		}

		st := bySource[e.Source]
		st.TotalRecords++
		if mayDup {
			st.BloomMayDuplicate++
		}
		bySource[e.Source] = st
	})
	if err != nil {
		return nil, err
	}
	bloomDuration := time.Since(start).Milliseconds()

	bloomMemory := 0
	if bf != nil {
		bloomMemory = bf.MemoryBytes()
	} else {
		bloomMemory = cf.MemoryBytes()
	}

	fpTable, err := ParamFPRows(cfg.ExpectedItems, DefaultRates)
	if err != nil {
		return nil, err
	}

	return &Report{
		TotalRecords:      total,
		BadLines:          len(badLines),
		BloomNew:          total - bloomDup,
		BloomMayDuplicate: bloomDup,
		BloomMemoryBytes:  bloomMemory,
		DurationMs:        bloomDuration,
		BloomDurationMs:   bloomDuration,
		FPTable:           fpTable,
		BySource:          bySource,
		InvalidSources:    uniqueStrings(badSources),
	}, nil
}

// Отбор уникальных невалидных источников
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Группировка данных по источнику
func BuildBySource(events []model.Event, hash string, mode string, mapMpde bool, fPR float64) (map[string]SourceStats, error) {
	bySource := make(map[string]SourceStats)
	grouped := make(map[string][]model.Event)
	for _, e := range events {
		grouped[e.Source] = append(grouped[e.Source], e)
	}
	for src, evs := range grouped {
		var bloomDup int
		var err1 error
		if mode == "bloom" {
			_, bloomDup, _, _, err1 = bloom.BloomFilter(evs, len(evs), hash, fPR)
		} else {
			_, bloomDup, _, _, err1 = bloom.CountingBloomFilter(evs, len(evs), hash, fPR)
		}
		if err1 != nil {
			return nil, err1
		}

		st := SourceStats{
			TotalRecords:      len(evs),
			BloomMayDuplicate: bloomDup,
		}
		if mapMpde {
			_, exactUnique, exactDup, _, _, err2 := bloom.MapFilter(evs)
			if err2 != nil {
				return nil, err2
			}
			estFP := bloomDup - exactDup
			if estFP < 0 {
				estFP = 0
			}
			st.ExactUnique = &exactUnique
			st.ExactDuplicates = &exactDup
			st.EstimatedFalsePositives = &estFP
		}
		bySource[src] = st
	}
	return bySource, nil
}

func SaveJSON(path string, report *Report) error {
	byteValue, err := json.MarshalIndent(report, "", " ")
	if err != nil {
		return fmt.Errorf("не удалось сериализовать отчёт: %w", err)
	}
	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("не удалось создать директорию для %s: %w", path, err)
	}
	err = os.WriteFile(path, byteValue, 0644)
	if err != nil {
		return fmt.Errorf("не удалось записать файл %s: %w", path, err)
	}
	return nil
}

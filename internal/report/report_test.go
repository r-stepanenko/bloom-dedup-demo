package report

import (
	"bloom-dedup-demo/internal/model"
	"fmt"
	"testing"
)

func TestBuildReport(t *testing.T) {

	events, badLines, badSources, err := model.ReadEvents("../../testdata/tests/event.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}

	report, err := BuildReport(events, badLines, badSources, "../../testdata/tests/bloom1.json", true)
	if err != nil {
		t.Fatalf("BuildReport вернул ошибку: %v", err)
	}

	if report.TotalRecords != 4 {
		t.Errorf("ожидали TotalRecords=4, получили %d", report.TotalRecords)
	}
	if *report.ExactUnique != 4 {
		t.Errorf("ожидали ExactUnique=4, получили %d", report.ExactUnique)
	}
	if *report.ExactDuplicates != 0 {
		t.Errorf("ожидали ExactDuplicates=0, получили %d", report.ExactDuplicates)
	}
	if report.BloomMemoryBytes <= 0 {
		t.Errorf("ожидали положительный BloomMemoryBytes, получили %d", report.BloomMemoryBytes)
	}
	if *report.ExactMapMemoryBytes <= 0 {
		t.Errorf("ожидали положительный ExactMapMemoryBytes, получили %d", report.ExactMapMemoryBytes)
	}
	if *report.MapDurationMs < 0 {
		t.Errorf("MapDurationMs не может быть отрицательным, получили %d", report.MapDurationMs)
	}
	if report.BloomDurationMs < 0 {
		t.Errorf("BloomDurationMs не может быть отрицательным, получили %d", report.BloomDurationMs)
	}
	if report.BySource == nil {
		t.Errorf("ожидали ненулевую карту BySource")
	}
	if report.InvalidSources == nil {
		t.Errorf("ожидали ненулевой срез InvalidSources")
	}
	fmt.Println(report)
}

func TestBuildReportCountingBloom(t *testing.T) {
	events, badLines, badSources, err := model.ReadEvents("../../testdata/tests/event3.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}

	report, err := BuildReport(events, badLines, badSources, "../../testdata/tests/bloom_counting.json", true)
	if err != nil {
		t.Fatalf("BuildReport вернул ошибку: %v", err)
	}

	if report.TotalRecords != 200 {
		t.Errorf("ожидали TotalRecords=200, получили %d", report.TotalRecords)
	}
	if report.BadLines != 0 {
		t.Errorf("ожидали BadLines=0, получили %d", report.BadLines)
	}
	if *report.ExactUnique != 101 {
		t.Errorf("ожидали ExactUnique=101, получили %d", report.ExactUnique)
	}
	if *report.ExactDuplicates != 99 {
		t.Errorf("ожидали ExactDuplicates=99, получили %d", report.ExactDuplicates)
	}
	if report.BloomNew+report.BloomMayDuplicate != report.TotalRecords {
		t.Errorf("ожидали %d, получили %d + %d",
			report.TotalRecords, report.BloomNew, report.BloomMayDuplicate)
	}
	if report.BloomMemoryBytes <= 0 {
		t.Errorf("ожидали положительный BloomMemoryBytes, получили %d", report.BloomMemoryBytes)
	}
	if report.BySource == nil {
		t.Errorf("ожидали ненулевую карту BySource")
	}
}

func TestBuildReportBloomWithoutMap(t *testing.T) {
	events, badLines, badSources, err := model.ReadEvents("../../testdata/tests/event3.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}

	report, err := BuildReport(events, badLines, badSources, "../../testdata/tests/bloom1.json", false)
	if err != nil {
		t.Fatalf("BuildReport вернул ошибку: %v", err)
	}

	if report.TotalRecords != len(events) {
		t.Errorf("ожидали TotalRecords=%d, получили %d", len(events), report.TotalRecords)
	}
	if report.ExactUnique != nil {
		t.Errorf("ожидали ExactUnique=0, получили %d", report.ExactUnique)
	}
	if report.ExactDuplicates != nil {
		t.Errorf("ожидали ExactDuplicates=0, получили %d", report.ExactDuplicates)
	}
	if report.ExactMapMemoryBytes != nil {
		t.Errorf("ожидали ExactMapMemoryBytes=0, получили %d", report.ExactMapMemoryBytes)
	}
	if report.MapDurationMs != nil {
		t.Errorf("ожидали MapDurationMs=0, получили %d", report.MapDurationMs)
	}
	if report.EstimatedFalsePositives != nil {
		t.Errorf("ожидали EstimatedFalsePositives=0, получили %d", report.EstimatedFalsePositives)
	}
	if report.RealFalsePositiveRate != nil {
		t.Errorf("ожидали RealFalsePositiveRate=0, получили %v", report.RealFalsePositiveRate)
	}
	if report.BloomNew+report.BloomMayDuplicate != report.TotalRecords {
		t.Errorf("BloomNew + BloomMayDuplicate должно быть %d, получили %d + %d",
			report.TotalRecords, report.BloomNew, report.BloomMayDuplicate)
	}
	for src, st := range report.BySource {
		if st.ExactUnique != nil || st.ExactDuplicates != nil || st.EstimatedFalsePositives != nil {
			t.Errorf("для source=%s в режиме no map поля exact должны быть null", src)
		}
	}
}

func TestBuildBySourceBloomWithMap(t *testing.T) {
	events, _, _, err := model.ReadEvents("../../testdata/tests/mixed_sources.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}

	stats, err := BuildBySource(events, "fnv64_double_hashing", "bloom", true, 0.01)
	if err != nil {
		t.Fatalf("BuildBySource вернул ошибку: %v", err)
	}

	if len(stats) == 0 {
		t.Fatal("ожидали ненулевую карту BySource")
	}

	for src, st := range stats {
		if src == "" {
			t.Errorf("пустой source")
		}
		if st.TotalRecords <= 0 {
			t.Errorf("для source=%s ожидали TotalRecords > 0, получили %d", src, st.TotalRecords)
		}
		if *st.ExactUnique+*st.ExactDuplicates != st.TotalRecords {
			t.Errorf("для source=%s ExactUnique + ExactDuplicates должно быть %d, получили %d + %d",
				src, st.TotalRecords, st.ExactUnique, st.ExactDuplicates)
		}
	}
}

func TestBuildBySourceCountingBloomWithoutMap(t *testing.T) {
	events, _, _, err := model.ReadEvents("../../testdata/tests/mixed_sources.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}

	stats, err := BuildBySource(events, "fnv64_double_hashing", "counting_bloom", false, 0.01)
	if err != nil {
		t.Fatalf("BuildBySource вернул ошибку: %v", err)
	}

	if len(stats) == 0 {
		t.Fatal("ожидали ненулевую карту BySource")
	}

	for src, st := range stats {
		if src == "" {
			t.Errorf("пустой source")
		}
		if st.TotalRecords <= 0 {
			t.Errorf("для source=%s ожидали TotalRecords > 0, получили %d", src, st.TotalRecords)
		}
		if st.ExactUnique != nil {
			t.Errorf("для source=%s ожидали ExactUnique=0, получили %d", src, st.ExactUnique)
		}
		if st.ExactDuplicates != nil {
			t.Errorf("для source=%s ожидали ExactDuplicates=0, получили %d", src, st.ExactDuplicates)
		}
		if st.EstimatedFalsePositives != nil {
			t.Errorf("для source=%s ожидали EstimatedFalsePositives=0, получили %d", src, st.EstimatedFalsePositives)
		}
	}
}

func TestUniqueStrings(t *testing.T) {
	in := []string{"a", "b", "a", "c", "b", "d"}
	out := uniqueStrings(in)

	if len(out) != 4 {
		t.Fatalf("ожидали 4 уникальные строки, получили %d", len(out))
	}

	expected := []string{"a", "b", "c", "d"}
	for i, s := range out {
		if s != expected[i] {
			t.Errorf("элемент %d не совпал", i)
		}
	}
}

func TestBuildReportStreaming(t *testing.T) {
	report, err := BuildReportStreaming("../../testdata/tests/event3.jsonl", true, "../../testdata/tests/bloom1.json")
	if err != nil {
		t.Fatalf("BuildReportStreaming вернул ошибку: %v", err)
	}
	if report.TotalRecords != 200 {
		t.Errorf("ожидали TotalRecords=200, получили %d", report.TotalRecords)
	}
	if report.BloomNew+report.BloomMayDuplicate != report.TotalRecords {
		t.Errorf("BloomNew + BloomMayDuplicate должно быть %d, получили %d + %d",
			report.TotalRecords, report.BloomNew, report.BloomMayDuplicate)
	}
	if report.BloomMemoryBytes <= 0 {
		t.Errorf("ожидали положительный BloomMemoryBytes, получили %d", report.BloomMemoryBytes)
	}
	totalBySource := 0
	for src, st := range report.BySource {
		totalBySource += st.TotalRecords
		if st.ExactUnique != nil || st.ExactDuplicates != nil || st.EstimatedFalsePositives != nil {
			t.Errorf("для source=%s в no map точные поля должны быть null", src)
		}
	}
	if totalBySource != report.TotalRecords {
		t.Errorf("сумма TotalRecords по источникам должна быть %d, получили %d", report.TotalRecords, totalBySource)
	}
}

func TestBuildReportStreamingBadFile(t *testing.T) {
	_, err := BuildReportStreaming("../../testdata/tests/no_file.jsonl", true, "../../testdata/tests/bloom1.json")
	if err == nil {
		t.Errorf("ожидали ошибку для несуществующего файла")
	}
}

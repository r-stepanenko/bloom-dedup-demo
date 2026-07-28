package report

import (
	"bloom-dedup-demo/internal/model"
	"testing"
)

func BenchmarkBuildReportWithMap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		events, badLines, badSources, err := model.ReadEvents("../../testdata/tests/event_large.jsonl", true)
		if err != nil {
			b.Fatal(err)
		}
		_, err = BuildReport(events, badLines, badSources, "../../testdata/tests/large_config.json", true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildReportStreaming(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := BuildReportStreaming("../../testdata/tests/event_large.jsonl", true, "../../testdata/tests/large_config.json")
		if err != nil {
			b.Fatal(err)
		}
	}
}

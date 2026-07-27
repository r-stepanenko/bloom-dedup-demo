package model

import (
	"path/filepath"
	"testing"
)

func TestGenerateEvents1(t *testing.T) {
	path := "../../testdata/tests/event1.jsonl"

	n := 100
	_ = GenerateEvents(path, n, 5, 0.1, 42)

	_, badLines, sour, err := ReadEvents(path, true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badLines) != 0 {
		t.Errorf("ожидали 0 битых строк, получили %v", badLines)
	}
	//if total != n {
	//	t.Errorf("ожидали %d событий, получили %d", n, total)
	//}
	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
}

func TestGenerateEvents2(t *testing.T) {
	path := "../../testdata/tests/event2.jsonl"

	n := 50
	_ = GenerateEvents(path, n, 1, 0.0, 7)

	events, badLines, sour, err := ReadEvents(path, true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badLines) != 0 {
		t.Errorf("ожидали 0 битых строк, получили %v", badLines)
	}
	//if total != n {
	//	t.Errorf("ожидали %d событий, получили %d", n, total)
	//}
	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
	for _, e := range events {
		if e.Source != "collector_01" {
			t.Errorf("при s=1 source должен быть collector_01, получено %q", e.Source)
		}
	}
}

func TestGenerateEvents3(t *testing.T) {
	path := "../../testdata/tests/event3.jsonl"

	n := 200
	_ = GenerateEvents(path, n, 20, 0.5, 123)

	_, badLines, sour, err := ReadEvents(path, true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badLines) != 0 {
		t.Errorf("ожидали 0 битых строк, получили %v", badLines)
	}
	//if total != n {
	//	t.Errorf("ожидали %d событий, получили %d", n, total)
	//}
	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
}

func TestGenerateInvalidPathEvents(t *testing.T) {
	path := ""
	n := 20
	err := GenerateEvents(path, n, 20, 0.5, 123)
	if err == nil {
		t.Errorf("ожидали ошибку аргумента, получили nil")
	}
}

func TestGenerateZeroNEvents(t *testing.T) {
	path := "../../testdata/tests/event7.jsonl"
	n := 0
	err := GenerateEvents(path, n, 20, 0.5, 123)
	if err == nil {
		t.Errorf("ожидали ошибку аргумента, получили nil")
	}
}

func TestGenerateInvalidRate(t *testing.T) {
	path := "../../testdata/tests/event7.jsonl"
	n := 20
	err := GenerateEvents(path, n, 20, 1.5, 123)
	if err == nil {
		t.Errorf("ожидали ошибку аргумента, получили nil")
	}
}

func TestGenerateEvents_1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event1.jsonl")

	n := 100
	err := GenerateEvents(path, n, 5, 0.1, 42)
	if err != nil {
		t.Fatalf("GenerateEvents вернул ошибку: %v", err)
	}

	events, badLines, sour, err := ReadEvents(path, true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badLines) != 0 {
		t.Errorf("ожидали 0 битых строк, получили %v", badLines)
	}
	if len(events) != n {
		t.Errorf("ожидали %d событий, получили %d", n, len(events))
	}
	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
}

func TestGenerateEvents_2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event2.jsonl")

	n := 50
	err := GenerateEvents(path, n, 1, 0.0, 7)
	if err != nil {
		t.Fatalf("GenerateEvents вернул ошибку: %v", err)
	}

	events, badLines, sour, err := ReadEvents(path, true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badLines) != 0 {
		t.Errorf("ожидали 0 битых строк, получили %v", badLines)
	}
	if len(events) != n {
		t.Errorf("ожидали %d событий, получили %d", n, len(events))
	}
	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
	for _, e := range events {
		if e.Source != "collector_01" {
			t.Errorf("при s=1 source должен быть collector_01, получено %q", e.Source)
		}
	}
}

func TestGenerateEvents_3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event3.jsonl")

	n := 200
	err := GenerateEvents(path, n, 20, 0.5, 123)
	if err != nil {
		t.Fatalf("GenerateEvents вернул ошибку: %v", err)
	}

	events, badLines, sour, err := ReadEvents(path, true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badLines) != 0 {
		t.Errorf("ожидали 0 битых строк, получили %v", badLines)
	}
	if len(events) != n {
		t.Errorf("ожидали %d событий, получили %d", n, len(events))
	}
	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
}

func TestGenerateEventsBadDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no_dir", "event.jsonl")
	err := GenerateEvents(path, 10, 5, 0.1, 42)
	if err == nil {
		t.Errorf("ожидали ошибку создания файла в несуществующей директории, получили nil")
	}
}

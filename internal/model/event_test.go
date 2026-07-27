package model

import "testing"

func TestReadEvents(t *testing.T) {
	events, badLines, sour, err := ReadEvents("../../testdata/tests/event.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}

	//if total != 4 {
	//	t.Errorf("ожидали 4 валидных события, получили %d", total)
	//}
	if len(badLines) != 1 || badLines[0] != 3 {
		t.Errorf("ожидали одну битую строку с номером 3, получили %v", badLines)
	}

	if len(sour) != 0 {
		t.Errorf("ожидали 0 невалидных источников, получили %v", len(sour))
	}
	for _, e := range events {
		if err := e.Validate(); err != nil {
			t.Errorf("событие %s не прошло валидацию: %v", e.EventID, err)
		}
	}
}

func TestReadBadEvents(t *testing.T) {
	_, badLines, sour, err := ReadEvents("../../testdata/tests/bad_lines.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents выдал ошибку: %v", err)
	}
	if len(badLines) != 9 {
		t.Errorf("ожидали 9 битых строк, получили %d", len(badLines))
	}
	//if total != 4 {
	//	t.Errorf("ожидали 4 валидных строк, получили %d", total)
	//}
	if len(sour) != 0 {

	}
}

func TestEventValidateSource(t *testing.T) {
	e := Event{Source: "bad"}
	valid, _ := e.ValidateSource()
	if valid {
		t.Errorf("ожидалась ошибка, получено значение true")
	}
}

func TestEventValidateEmptySource(t *testing.T) {
	e := Event{Source: ""}
	valid, _ := e.ValidateSource()
	if !valid {
		t.Errorf("пустой source должен быть валидным, получено invalid")
	}
}

func TestReadEventsStrict(t *testing.T) {
	_, _, badSrc, err := ReadEvents("../../testdata/tests/mixed_sources.jsonl", true)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badSrc) == 0 {
		t.Errorf("ожидали хотя бы один невалидный source")
	}
	//if total != 1 {
	//	t.Errorf("Должно считаться 1 событие, считалось %d", total)
	//}
}

func TestReadEventsNotStrict(t *testing.T) {
	_, _, badSrc, err := ReadEvents("../../testdata/tests/mixed_sources.jsonl", false)
	if err != nil {
		t.Fatalf("ReadEvents вернул ошибку: %v", err)
	}
	if len(badSrc) == 0 {
		t.Errorf("ожидали хотя бы один невалидный source")
	}
	//if total != 4 {
	//	t.Errorf("Должно считаться 4 события, считалось %d", total)
	//}
}

func TestEventValidateHash(t *testing.T) {
	base := Event{EventID: "evt_000001", Seq: 1}

	e := base
	e.EventHash = "0123456789abcdef"
	err := e.Validate()
	if err != nil {
		t.Errorf("валидный хеш не должен давать ошибку: %v", err)
	}

	e = base
	e.EventHash = "0123456789abcdef0123456789ABCDEF"
	err = e.Validate()
	if err != nil {
		t.Errorf("валидный хеш длиной 32 не должен давать ошибку: %v", err)
	}

	e = base
	e.EventHash = "0123456789abcdeg"
	if err := e.Validate(); err == nil {
		t.Errorf("хеш с неправильным символом должен давать ошибку")
	}

	e = base
	e.EventHash = "0123456789abcde"
	err = e.Validate()
	if err == nil {
		t.Errorf("хеш длиной 15 должен давать ошибку")
	}
}

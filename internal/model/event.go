package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Event struct {
	Seq       int    `json:"seq"`
	EventID   string `json:"event_id"`
	EventHash string `json:"event_hash"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
}

// Валидация
func (e *Event) Validate() error {
	if e.Seq <= 0 {
		return fmt.Errorf("seq должен быть больше 0, получено %d", e.Seq)
	}
	if e.EventID == "" {
		return fmt.Errorf("event_id обязателен и не может быть пустым")
	}
	if len(e.EventID) < 10 {
		return fmt.Errorf("event_id должен быть не менее 10 символов, получено %q длиной %d", e.EventID, len(e.EventID))
	}
	if e.EventID[:4] != "evt_" {
		return fmt.Errorf("event_id должен начинаться с 'evt_', получено %q", e.EventID)
	}
	if e.EventHash == "" {
		return fmt.Errorf("event_hash обязателен и не может быть пустым")
	}
	if len(e.EventHash) != 16 && len(e.EventHash) != 32 {
		return fmt.Errorf("event_hash должен быть 16 или 32 hex-символа, получено %d символов", len(e.EventHash))
	}
	if !isHex(e.EventHash) {
		return fmt.Errorf("event_hash должен состоять только из hex-символов, получено %q", e.EventHash)
	}
	if len(e.EventID) > 256 {
		return fmt.Errorf("event_id превышает допустимую длину %d символов", 256)
	}
	if len(e.Source) > 256 {
		return fmt.Errorf("source превышает допустимую длину %d символов", 256)
	}
	return nil
}

// Проверка на hex симолы
func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// Валидация источника
func (e *Event) ValidateSource() (bool, string) {
	if e.Source == "" {
		return true, ""
	}
	if len(e.Source) != 12 {
		return false, e.Source
	}
	if e.Source[:10] != "collector_" {
		return false, e.Source
	}
	i := e.Source[10:]
	iInt, err := strconv.Atoi(i)
	if err != nil || iInt < 1 || iInt > 99 {
		return false, e.Source
	}
	return true, ""
}

func (e *Event) ValidateTimestamp() bool {
	if e.Timestamp != "" {
		if _, err := time.Parse(time.RFC3339, e.Timestamp); err != nil {
			return false
		}
	}
	return true
}

// Потоковое считывание файла входных данных построчно
func StreamEvents(path string, fs bool, fn func(Event)) ([]int, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось открыть файл %s: %w", path, err)
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	badLines := []int{}
	badSources := []string{}
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			badLines = append(badLines, lineNum)
			continue
		}
		err1 := event.Validate()
		if err1 != nil {
			badLines = append(badLines, lineNum)
			continue
		}
		flag, str := event.ValidateSource()
		flag1 := event.ValidateTimestamp()
		if !flag {
			badSources = append(badSources, str)
		}

		if fs {
			if flag && flag1 {
				fn(event)
			} else {
				badLines = append(badLines, lineNum)
			}
		} else {
			if !flag {
				event.Source = ""
			}
			if !flag1 {
				event.Timestamp = ""
			}
			fn(event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("ошибка чтения файла %s: %w", path, err)
	}

	err2 := file.Close()
	if err2 != nil {
		return nil, nil, fmt.Errorf("ошибка закрытия файла %s: %w", path, err2)
	}

	return badLines, badSources, nil
}

// Считывание файла входных данных построчно
// Получает: путь к файлу, флаг пропуска невалидных данных (source, time)
// Возвращает: массив распарсенных данных, массив пропущенных битых и невалидных строк,
// список невалидных источников, ошибку
// Если какого-то элемента выходных данных нет - возвращает nil
func ReadEvents(path string, fs bool) ([]Event, []int, []string, error) {
	events := []Event{}
	badLines, badSources, err := StreamEvents(path, fs, func(e Event) {
		events = append(events, e)
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return events, badLines, badSources, nil
}

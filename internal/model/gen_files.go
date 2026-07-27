package model

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

func generateHash(key string) string {
	hash := sha256.New()
	hash.Write([]byte(key))
	hexString := hex.EncodeToString(hash.Sum(nil))
	return hexString[:16]
}

type genEvent struct {
	EventID   string
	EventHash string
}

// Генератор событий
// Вход: путь и название файла, общее кол-во, источники, вероятность дублей, seed
func GenerateEvents(path string, n int, s int, duble float64, seed int64) error {
	if path == "" {
		return fmt.Errorf("неправильный аргумент path")
	} else if n == 0 {
		return fmt.Errorf("неправильный аргумент n")
	} else if duble < 0.0 || duble > 0.9 {
		return fmt.Errorf("неправильный аргумент duble")
	} else if s < 0 || s > 99 {
		return fmt.Errorf("неправильный аргумент s")
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("не удалось создать файл %s: %w", path, err)
	}
	writer := bufio.NewWriter(file)

	r := rand.New(rand.NewSource(seed))
	baseTime := time.Date(2026, 7, 7, 12, 45, 0, 0, time.UTC)

	history := []genEvent{}

	for i := 0; i < n; i++ {
		var evtId, hash string

		if len(history) > 0 && r.Float64() < duble {
			idx := r.Intn(len(history))
			evtId = history[idx].EventID
			hash = history[idx].EventHash
		} else {
			evtId = "evt_"
			evt := strconv.Itoa(i + 1)
			for len(evt) < 6 {
				evt = "0" + evt
			}
			evtId = evtId + evt

			hash = generateHash(evtId + "ev123" + strconv.Itoa(i+1))
			history = append(history, genEvent{EventID: evtId, EventHash: hash})
		}

		source := ""
		if s != 0 {
			source = "collector_"
			num := 1 + r.Intn(s)
			if num < 10 {
				source = source + "0" + strconv.Itoa(num)
			} else {
				source = source + strconv.Itoa(num)
			}
		}

		ts := baseTime.Add(time.Duration(i*5) * time.Second)

		event := Event{
			Seq:       i + 1,
			EventID:   evtId,
			EventHash: hash,
			Source:    source,
			Timestamp: ts.Format(time.RFC3339),
		}

		data, err := json.Marshal(event)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка преобразования (marshal):", err)
			continue
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("ошибка записи события: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("ошибка записи в файл: %w", err)
	}
	err1 := file.Close()
	if err1 != nil {
		return fmt.Errorf("ошибка закрытия файла: %w", err1)
	}
	return nil
}

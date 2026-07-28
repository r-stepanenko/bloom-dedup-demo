package report

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

func SaveMarkdown(path string, report *Report) error {
	if path == "" {
		return fmt.Errorf("неправильный аргумент path")
	}
	if report == nil {
		return fmt.Errorf("отчёт не может быть nil")
	}

	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("не удалось создать директорию для %s: %w", path, err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("не удалось создать файл %s: %w", path, err)
	}

	writer := bufio.NewWriter(file)

	if _, err := writer.WriteString("# Отчёт о выполненной фильтрации"); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	if report.EstimatedFalsePositives == nil && report.RealFalsePositiveRate == nil {
		if _, err := writer.WriteString("# Map фильтр не запускался, его параметры отсутствуют"); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
	}
	if _, err := writer.WriteString("## Общая статистика"); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}

	exUn := "не измерялось"
	if report.ExactUnique != nil {
		exUn = strconv.Itoa(*report.ExactUnique)
	}

	exDup := "не измерялось"
	if report.ExactDuplicates != nil {
		exDup = strconv.Itoa(*report.ExactDuplicates)
	}

	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	if _, err := writer.WriteString("- Общее количество строк: " + strconv.Itoa(report.TotalRecords)); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Выявлено битых/невалидных строк: " + strconv.Itoa(report.BadLines)); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Точное количество уникальных событий: " + exUn); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Точное количество дубликатов: " + exDup); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Уникальные события по фильтру Блума: " + strconv.Itoa(report.BloomNew)); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Дубликаты по фильтру Блума: " + strconv.Itoa(report.BloomMayDuplicate)); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	estFPStr := "не измерялось"
	if report.EstimatedFalsePositives != nil {
		estFPStr = strconv.Itoa(*report.EstimatedFalsePositives)
	}
	fpRateStr := "не измерялась"
	if report.RealFalsePositiveRate != nil {
		fpRateStr = strconv.FormatFloat(*report.RealFalsePositiveRate, 'f', 10, 64)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Число случаев, когда фильтр Блума ошибочно счёл новый элемент дублем: " + estFPStr); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Измеренная доля ложных срабатываний: " + fpRateStr); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Количество байт для фильтра Блума: " + strconv.Itoa(report.BloomMemoryBytes)); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	bloomMiB := float64(report.BloomMemoryBytes) / 1024.0 / 1024.0
	exactMiB := "не измерялось"
	if report.ExactMapMemoryBytes != nil {
		exactMiB = strconv.FormatFloat(float64(*report.ExactMapMemoryBytes)/1024.0/1024.0, 'f', 10, 64)
	}
	exMem := "не измерялось"
	if report.ExactMapMemoryBytes != nil {
		exMem = strconv.Itoa(*report.ExactMapMemoryBytes)
	}
	exDur := "не измерялось"
	if report.MapDurationMs != nil {
		exDur = strconv.Itoa(int(*report.MapDurationMs))
	}
	duration := report.BloomDurationMs
	if report.MapDurationMs != nil {
		duration += *report.MapDurationMs
	}
	if bloomMiB != 0.0 {
		if _, err := writer.WriteString(fmt.Sprintf("- Размер фильтра Блума: %.2f Mб", bloomMiB)); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
	}
	if _, err := writer.WriteString("- Количество байт для точного сравнения: " + exMem); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString(fmt.Sprintf("- Размер структуры точного сравнения: %s Mб", exactMiB)); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	if _, err := writer.WriteString("- Время выполнения анализа (мс): " + strconv.Itoa(int(duration))); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	if _, err := writer.WriteString("- Время выполнения фильтра Блума (мс): " + strconv.Itoa(int(report.BloomDurationMs))); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if _, err := writer.WriteString("- Время выполнения точного сравнения (мс): " + exDur); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	if _, err := writer.WriteString("## Статистика по источникам"); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	sources := make([]string, 0, len(report.BySource))
	for source := range report.BySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)

	for _, source := range sources {
		bs := report.BySource[source]
		if _, err := writer.WriteString("### " + source); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}

		if _, err := writer.WriteString("- Общее количество строк: " + strconv.Itoa(bs.TotalRecords)); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		bsExUn := "не измерялось"
		if bs.ExactUnique != nil {
			bsExUn = strconv.Itoa(*bs.ExactUnique)
		}
		bsExDup := "не измерялось"
		if bs.ExactDuplicates != nil {
			bsExDup = strconv.Itoa(*bs.ExactDuplicates)
		}
		bsEstFP := "не измерялось"
		if bs.EstimatedFalsePositives != nil {
			bsEstFP = strconv.Itoa(*bs.EstimatedFalsePositives)
		}
		if _, err := writer.WriteString("- Точное количество уникальных событий: " + bsExUn); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		if _, err := writer.WriteString("- Точное количество дубликатов: " + bsExDup); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		if _, err := writer.WriteString("- Дубликаты по фильтру Блума: " + strconv.Itoa(bs.BloomMayDuplicate)); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		if _, err := writer.WriteString("- Число случаев, когда фильтр Блума ошибочно счёл новый элемент дублем: " + bsEstFP); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
	}

	if len(report.FPTable) > 0 {
		_, err := writer.WriteString("## Сравнение значений false_positive_rate\n\n")
		if err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		_, err = writer.WriteString("| p | m (бит) | k | Память (байт) | Дубли (Блум) | Ложные срабатывания | Реальный FP rate |\n" +
			"|-----------|-----------|-----------|-----------|-----------|-----------|-----------|\n")
		if err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		for _, row := range report.FPTable {
			dup := "не измерялось"
			if row.BloomMayDuplicate != nil {
				dup = strconv.Itoa(*row.BloomMayDuplicate)
			}
			estFP := "не измерялось"
			if row.EstimatedFalsePositives != nil {
				estFP = strconv.Itoa(*row.EstimatedFalsePositives)
			}
			fpRate := "не измерялась"
			if row.RealFalsePositiveRate != nil {
				fpRate = strconv.FormatFloat(*row.RealFalsePositiveRate, 'f', 6, 64)
			}
			line := fmt.Sprintf("| %g | %d | %d | %d | %s | %s | %s |\n",
				row.FalsePositiveRate, row.MBits, row.KHashes, row.MemoryBytes, dup, estFP, fpRate)
			if _, err := writer.WriteString(line); err != nil {
				return fmt.Errorf("ошибка записи строки: %w", err)
			}
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
	}

	if _, err := writer.WriteString("## Невалидные источники"); err != nil {
		return fmt.Errorf("ошибка записи строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("ошибка записи перевода строки: %w", err)
	}

	if len(report.InvalidSources) == 0 {
		if _, err := writer.WriteString("Все источники были валидными."); err != nil {
			return fmt.Errorf("ошибка записи строки: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("ошибка записи перевода строки: %w", err)
		}
	} else {
		for _, s := range report.InvalidSources {
			if _, err := writer.WriteString("- " + s); err != nil {
				return fmt.Errorf("ошибка записи строки: %w", err)
			}
			if err := writer.WriteByte('\n'); err != nil {
				return fmt.Errorf("ошибка записи перевода строки: %w", err)
			}
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("ошибка записи в файл: %w", err)
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("ошибка закрытия файла: %w", err)
	}
	return nil
}

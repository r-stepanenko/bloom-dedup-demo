package main

import (
	"bloom-dedup-demo/internal/model"
	report2 "bloom-dedup-demo/internal/report"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("нужна подкоманда: generate, run или bench")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "generate":
		genCmd := flag.NewFlagSet("generate", flag.ExitOnError)
		countFlag := genCmd.Int("count", 0, "общее число записей")
		duplicateRatioFlag := genCmd.Float64("duplicate-ratio", 0.0, "доля дублей")
		outFlag := genCmd.String("out", "", "файл вывода")
		seedFlag := genCmd.Int64("seed", 0, "Seed генератора")
		sourcesFlag := genCmd.Int("sources", 1, "число источников")
		genCmd.Parse(os.Args[2:])
		if *countFlag <= 0 {
			fmt.Fprintln(os.Stderr, "count должен быть больше 0")
			os.Exit(1)
		}
		if *duplicateRatioFlag < 0 || *duplicateRatioFlag > 0.9 {
			fmt.Fprintln(os.Stderr, "duplicate-ratio от 0 до 0.9")
			os.Exit(1)
		}
		if *outFlag == "" {
			fmt.Fprintln(os.Stderr, "путь не может быть пустым")
			os.Exit(1)
		}
		if *sourcesFlag <= 0 || *sourcesFlag > 99 {
			fmt.Fprintln(os.Stderr, "источники должны быть от 1 до 99")
			os.Exit(1)
		}
		//fmt.Println(*countFlag, *duplicateRatioFlag, *outFlag, *seedFlag, *sourcesFlag)
		err := model.GenerateEvents(*outFlag, *countFlag, *sourcesFlag, *duplicateRatioFlag, *seedFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		in := runCmd.String("in", "", "входной файл")
		cfg := runCmd.String("config", "", "файл конфигурации")
		outRes := runCmd.String("out", "", "файл JSON отчёта")
		reportFile := runCmd.String("report", "", "файл MD отчёта")
		sourcesBoolFlag := runCmd.Bool("fls", true, "флаг пропуска событий с невалидными источником и датой (true - пропуск)")
		exactCompare := runCmd.Bool("exact-compare", true, "флаг отключения точного сравнения (false - без map")
		runCmd.Parse(os.Args[2:])
		if *in == "" {
			fmt.Fprintln(os.Stderr, "путь файла событий не может быть пустым")
			os.Exit(1)
		}
		if *cfg == "" {
			fmt.Fprintln(os.Stderr, "путь файла конфигурации не может быть пустым")
			os.Exit(1)
		}
		if *outRes == "" {
			fmt.Fprintln(os.Stderr, " путь выходного файла не может быть пустым при --exact-compare=true")
			os.Exit(1)
		}
		if filepath.Ext(*outRes) != ".json" {
			fmt.Fprintln(os.Stderr, "файл --out должен быть с расширением json")
			os.Exit(1)
		}
		if *reportFile != "" && filepath.Ext(*reportFile) != ".md" {
			fmt.Fprintln(os.Stderr, "файл --report должен быть с расширением md")
			os.Exit(1)
		}
		events, badLines, badSources, err := model.ReadEvents(*in, *sourcesBoolFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		var rep *report2.Report
		if *exactCompare {
			rep, err = report2.BuildReport(events, badLines, badSources, *cfg, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else {
			var err1 error
			rep, err1 = report2.BuildReportStreaming(*in, *sourcesBoolFlag, *cfg)
			if err1 != nil {
				fmt.Fprintln(os.Stderr, err1)
				os.Exit(1)
			}
		}
		err = report2.SaveJSON(*outRes, rep)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *reportFile != "" {
			err = report2.SaveMarkdown(*reportFile, rep)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}

	case "bench":
		benchCmd := flag.NewFlagSet("bench", flag.ExitOnError)
		in := benchCmd.String("in", "", "входной файл")
		cfg := benchCmd.String("config", "", "файл конфигурации")
		sourcesBoolFlag := benchCmd.Bool("fls", true, "флаг пропуска событий с невалидными источником и датой (true - пропуск)")

		benchCmd.Parse(os.Args[2:])
		if *in == "" {
			fmt.Fprintln(os.Stderr, "путь файла событий не может быть пустым")
			os.Exit(1)
		}
		if *cfg == "" {
			fmt.Fprintln(os.Stderr, "путь файла конфигурации не должен быть пустым")
			os.Exit(1)
		}
		events, badLines, badSources, err := model.ReadEvents(*in, *sourcesBoolFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		rep, err := report2.BuildReport(events, badLines, badSources, *cfg, true)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		bloomMiB := float64(rep.BloomMemoryBytes) / 1024.0 / 1024.0
		exactMiB := float64(*rep.ExactMapMemoryBytes) / 1024.0 / 1024.0

		memoryRatio := 0.0
		if rep.BloomMemoryBytes > 0 {
			memoryRatio = float64(*rep.ExactMapMemoryBytes) / float64(rep.BloomMemoryBytes)
		}
		speedRatio := 0.0
		if rep.BloomDurationMs > 0 {
			speedRatio = float64(*rep.MapDurationMs) / float64(rep.BloomDurationMs)
		}

		mapLinesPerSec := 0.0
		if *rep.MapDurationMs > 0 {
			mapLinesPerSec = float64(rep.TotalRecords) * 1000.0 / float64(*rep.MapDurationMs)
		}

		bloomLinesPerSec := 0.0
		if rep.BloomDurationMs > 0 {
			bloomLinesPerSec = float64(rep.TotalRecords) * 1000.0 / float64(rep.BloomDurationMs)
		}
		estFP := 0
		if rep.EstimatedFalsePositives != nil {
			estFP = *rep.EstimatedFalsePositives
		}
		fpRate := 0.0
		if rep.RealFalsePositiveRate != nil {
			fpRate = *rep.RealFalsePositiveRate
		}

		fmt.Println("--- BENCH ---")
		fmt.Println()

		fmt.Printf("%-28s %s\n", "Входной файл:", *in)
		fmt.Printf("%-28s %s\n", "Файл конфигурации:", *cfg)
		fmt.Printf("%-28s %t\n", "Флаг пропуска:", *sourcesBoolFlag)
		fmt.Println()

		fmt.Println("--- Метрики ---")
		fmt.Println()
		fmt.Printf("%-40s %-18s %-18s\n", "Метрика", "Точное сравнение", "Фильтр Блума")
		fmt.Printf("%-40s %-18s %-18s\n", "----------------------------------------", "------------------", "------------------")
		fmt.Printf("%-40s %-18d %-18d\n", "Уникальные", *rep.ExactUnique, rep.BloomNew)
		fmt.Printf("%-40s %-18d %-18d\n", "Дубликаты", *rep.ExactDuplicates, rep.BloomMayDuplicate)
		fmt.Printf("%-40s %-18d %-18d\n", "Ложные срабатывания", 0, estFP)
		fmt.Printf("%-40s %-18.10f %-18.10f\n", "Ложное срабатывание rate", 0.0, fpRate)
		fmt.Printf("%-40s %-18d %-18d\n", "Память, байт", *rep.ExactMapMemoryBytes, rep.BloomMemoryBytes)
		fmt.Printf("%-40s %-18.2f %-18.2f\n", "Память, Мб", exactMiB, bloomMiB)
		fmt.Printf("%-40s %-18d %-18d\n", "Время, мс", *rep.MapDurationMs, rep.BloomDurationMs)
		fmt.Printf("%-40s %-18.2f %-18.2f\n", "Строк в секунду", mapLinesPerSec, bloomLinesPerSec)
		fmt.Println()

		fmt.Printf("%-40s %.2f раза\n", "Превышение памяти map над bloom:", memoryRatio)
		fmt.Printf("%-40s %.2f раза\n", "Замедление map относительно bloom:", speedRatio)

	case "fptable":
		fpCmd := flag.NewFlagSet("fptable", flag.ExitOnError)
		in := fpCmd.String("in", "", "входной файл")
		ratesFlag := fpCmd.String("rates", "0.1,0.05,0.01,0.001", "список false_positive_rate через запятую")
		hashFlag := fpCmd.String("hash", "f64", "способ хеширования: f64 - fnv64_double_hashing или s256 - sha256_slices")
		sourcesBoolFlag := fpCmd.Bool("fls", true, "флаг пропуска событий с невалидными источником и датой (true - пропуск)")
		fpCmd.Parse(os.Args[2:])
		if *in == "" {
			fmt.Fprintln(os.Stderr, "путь файла событий не может быть пустым")
			os.Exit(1)
		}
		if *hashFlag != "f64" && *hashFlag != "s256" {
			fmt.Fprintln(os.Stderr, "поддерживаются hash: f64, s256")
			os.Exit(1)
		}
		var rates []float64
		for _, s := range strings.Split(*ratesFlag, ",") {
			p, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				fmt.Fprintln(os.Stderr, "некорректное значение rate:", s)
				os.Exit(1)
			}
			rates = append(rates, p)
		}
		events, _, _, err := model.ReadEvents(*in, *sourcesBoolFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		err = report2.BuildFPTable(events, len(events), *hashFlag, rates)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Println("неизвестная подкоманда:", os.Args[1])
		os.Exit(1)
	}

}

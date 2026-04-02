//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type BenchmarkConfig struct {
	Name        string
	Description string
	Tests       []string
	Duration    time.Duration
	BenchTime   string
}

var configs = []BenchmarkConfig{
	{
		Name:        "quick",
		Description: "快速基准测试（适合日常开发）",
		Duration:    10 * time.Second,
		BenchTime:   "1s",
		Tests: []string{
			"BenchmarkActorTell$",
			"BenchmarkGoringQueuePushPop$",
			"BenchmarkMPSCQueuePushPop$",
		},
	},
	{
		Name:        "standard",
		Description: "标准基准测试（包含核心功能）",
		Duration:    30 * time.Second,
		BenchTime:   "2s",
		Tests: []string{
			"BenchmarkActorTell",
			"BenchmarkActorTellParallel",
			"BenchmarkActorContextAsk",
			"BenchmarkGoringQueue",
			"BenchmarkMPSCQueue",
		},
	},
	{
		Name:        "full",
		Description: "完整基准测试（所有场景）",
		Duration:    2 * time.Minute,
		BenchTime:   "5s",
		Tests: []string{
			"Benchmark",
			"TestStress",
		},
	},
	{
		Name:        "throughput",
		Description: "吞吐量专项测试",
		Duration:    1 * time.Minute,
		BenchTime:   "3s",
		Tests: []string{
			"BenchmarkActorTellStress",
			"BenchmarkActorTellParallelWithWorkerPool",
			"TestStressHighThroughput",
		},
	},
	{
		Name:        "latency",
		Description: "延迟专项测试",
		Duration:    45 * time.Second,
		BenchTime:   "2s",
		Tests: []string{
			"BenchmarkActorAskLatency",
			"BenchmarkActorContextAsk",
			"BenchmarkActorContextAskWithWorkerPool",
		},
	},
	{
		Name:        "stress",
		Description: "压力测试（长时间运行）",
		Duration:    5 * time.Minute,
		BenchTime:   "10s",
		Tests: []string{
			"TestStress",
		},
	},
	{
		Name:        "memory",
		Description: "内存相关测试",
		Duration:    30 * time.Second,
		BenchTime:   "3s",
		Tests: []string{
			"BenchmarkActorSpawn",
			"BenchmarkActorStop",
			"TestStressMemoryLeak",
		},
	},
	{
		Name:        "queue",
		Description: "队列性能测试",
		Duration:    20 * time.Second,
		BenchTime:   "2s",
		Tests: []string{
			"BenchmarkGoringQueue",
			"BenchmarkMPSCQueue",
		},
	},
}

func main() {
	listConfigs := flag.Bool("list", false, "列出所有可用的测试配置")
	configName := flag.String("config", "standard", "测试配置名称")
	help := flag.Bool("help", false, "显示帮助信息")
	flag.Parse()

	if *listConfigs {
		printConfigList()
		return
	}

	if *help {
		printHelp()
		return
	}

	// 查找配置
	var config *BenchmarkConfig
	for i, cfg := range configs {
		if cfg.Name == *configName {
			config = &configs[i]
			break
		}
	}

	if config == nil {
		fmt.Fprintf(os.Stderr, "未找到配置 '%s'\n", *configName)
		fmt.Fprintf(os.Stderr, "使用 -list 查看所有可用配置\n")
		os.Exit(1)
	}

	// 运行测试
	fmt.Printf("===========================================\n")
	fmt.Printf("运行配置: %s\n", config.Name)
	fmt.Printf("描述: %s\n", config.Description)
	fmt.Printf("预计时长: %v\n", config.Duration)
	fmt.Printf("===========================================\n\n")

	err := runBenchmark(*config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "运行测试失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n测试完成！\n")
}

func runBenchmark(config BenchmarkConfig) error {
	for _, testPattern := range config.Tests {
		fmt.Printf("运行测试模式: %s\n", testPattern)
		fmt.Println(strings.Repeat("-", 50))

		args := []string{
			"test",
			"-bench=" + testPattern,
			"-benchtime=" + config.BenchTime,
			"-benchmem",
			"-run=^$",
			"./...",
		}

		cmd := exec.Command("go", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		start := time.Now()
		err := cmd.Run()
		elapsed := time.Since(start)

		fmt.Printf("耗时: %v\n", elapsed)
		fmt.Println()

		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 部分测试失败: %v\n", err)
		}
	}

	return nil
}

func printConfigList() {
	fmt.Println("可用的测试配置:")
	fmt.Println("==================")
	for _, cfg := range configs {
		fmt.Printf("  %-15s - %s\n", cfg.Name, cfg.Description)
		fmt.Printf("                    时长: %v\n", cfg.Duration)
		fmt.Println()
	}
}

func printHelp() {
	fmt.Println("基准测试运行工具")
	fmt.Println("=================")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  go run benchmark_runner.go [选项]")
	fmt.Println()
	fmt.Println("选项:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  go run benchmark_runner.go -config quick")
	fmt.Println("  go run benchmark_runner.go -config full")
	fmt.Println("  go run benchmark_runner.go -list")
	fmt.Println()
	fmt.Println("直接运行:")
	fmt.Println("  go test -bench=. ./tests/")
	fmt.Println()
}

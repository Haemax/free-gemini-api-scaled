package api

import (
	"fmt"
	"goapi/gemini"
	"log"
	"sync"
	"time"
)

type WorkloadType string

const (
	WorkloadCode  WorkloadType = "Code Generation"
	WorkloadText  WorkloadType = "Text Generation"
	WorkloadDoc   WorkloadType = "Document Gen"
	WorkloadImage WorkloadType = "Image Generation"
	WorkloadMusic WorkloadType = "Music Generation"
)

type WorkloadTest struct {
	Type   WorkloadType
	Prompt string
	Tool   string
}

type ComprehensiveBenchmarkResult struct {
	Model            string
	Workload         WorkloadType
	TimeToFirstToken time.Duration
	TotalTime        time.Duration
	TokensPerSec     float64
	Success          bool
	Error            error
}

func RunBenchmark(cookiesFile string) {
	client, err := gemini.NewClient(cookiesFile)
	if err != nil {
		log.Fatalf("❌ Failed to initialize Gemini client for benchmark: %v", err)
	}

	if err := client.InitSession(); err != nil {
		log.Printf("⚠️ Session init warning: %v", err)
	}

	log.Println("🔍 Probing account endpoint for active models...")
	discovered, err := gemini.DiscoverAccountModels(client)
	if err != nil {
		log.Printf("⚠️ Discovery probe returned error: %v", err)
	} else {
		log.Printf("✨ Discovered %d model configurations from endpoint probe", len(discovered))
	}

	allModels := gemini.GetModelRegistry().GetAll()
	fmt.Printf("\n🤖 Registered models available for benchmark (%d total):\n", len(allModels))
	for _, m := range allModels {
		fmt.Printf("  • %-32s [%s] (MinTier: %s)\n", m.ID, m.DisplayName, m.MinTier)
	}
	fmt.Println()

	workloads := []WorkloadTest{
		{
			Type:   WorkloadCode,
			Prompt: "Write a fast standard library HTTP server in Go with graceful shutdown.",
			Tool:   "",
		},
		{
			Type:   WorkloadText,
			Prompt: "Provide a concise analysis of modern distributed systems consensus algorithms.",
			Tool:   "",
		},
		{
			Type:   WorkloadDoc,
			Prompt: "Generate a technical specification outline in Markdown format for an API gateway.",
			Tool:   "",
		},
		{
			Type:   WorkloadImage,
			Prompt: "A futuristic cybernetic city at dusk with glowing neon lights.",
			Tool:   "image_gen",
		},
		{
			Type:   WorkloadMusic,
			Prompt: "An upbeat atmospheric synthwave track with retro synth melodies.",
			Tool:   "music_gen",
		},
	}

	var results []ComprehensiveBenchmarkResult
	var wg sync.WaitGroup
	var mu sync.Mutex

	fmt.Printf("🚀 Starting Multi-Workload Benchmark across %d models using [%s]...\n\n", len(allModels), cookiesFile)

	for _, modelCfg := range allModels {
		for _, wl := range workloads {
			wg.Add(1)
			go func(m gemini.ModelConfig, test WorkloadTest) {
				defer wg.Done()

				start := time.Now()
				var ttft time.Duration
				firstChunkReceived := false
				tokenCount := 0
				var runErr error

				if test.Tool == "" {
					_, runErr = client.AskStreamModel(test.Prompt, m.ID, func(chunk string) {
						if !firstChunkReceived {
							ttft = time.Since(start)
							firstChunkReceived = true
						}
						tokenCount += len(chunk) / 4
					})
				} else {
					resp, err := client.AskWithToolModel(test.Prompt, test.Tool, m.ID)
					runErr = err
					if resp != nil {
						ttft = time.Duration(resp.Elapsed * float64(time.Second))
						tokenCount = len(resp.Text) / 4
					}
				}

				totalTime := time.Since(start)
				var tps float64
				if totalTime.Seconds() > 0 {
					tps = float64(tokenCount) / totalTime.Seconds()
				}

				mu.Lock()
				results = append(results, ComprehensiveBenchmarkResult{
					Model:            m.ID,
					Workload:         test.Type,
					TimeToFirstToken: ttft,
					TotalTime:        totalTime,
					TokensPerSec:     tps,
					Success:          runErr == nil,
					Error:            runErr,
				})
				mu.Unlock()
			}(modelCfg, wl)
		}
	}

	wg.Wait()

	fmt.Println("\n=========================================================================================================")
	fmt.Println("                               📊 MULTI-WORKLOAD BENCHMARK SUMMARY                                      ")
	fmt.Println("=========================================================================================================")
	fmt.Printf("%-32s | %-18s | %-12s | %-12s | %-10s | %-8s\n", "MODEL ID", "WORKLOAD", "TTFT", "TOTAL TIME", "TPS", "STATUS")
	fmt.Println("---------------------------------------------------------------------------------------------------------")

	for _, r := range results {
		status := "✅ OK"
		if !r.Success {
			status = "❌ FAIL"
		}
		fmt.Printf("%-32s | %-18s | %-12v | %-12v | %-10.2f | %-8s\n",
			r.Model, r.Workload, r.TimeToFirstToken.Round(time.Millisecond), r.TotalTime.Round(time.Millisecond), r.TokensPerSec, status)
		if r.Error != nil {
			fmt.Printf("   └── Error: %v\n", r.Error)
		}
	}
	fmt.Println("=========================================================================================================\n")
}

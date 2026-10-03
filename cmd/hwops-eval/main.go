// hwops-eval renders the frozen QA baseline produced by the public HTTP suite.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"hwops/internal/evaluation"
)

func main() {
	input := flag.String("input", "", "QA baseline JSON from TestQABaseline")
	output := flag.String("output", "", "Markdown report path")
	flag.Parse()
	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "-input and -output are required")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*input)
	var report evaluation.Baseline
	if err == nil {
		err = json.Unmarshal(raw, &report)
	}
	var text string
	if err == nil {
		text, err = report.Markdown()
	}
	if err == nil {
		err = os.WriteFile(*output, []byte(text), 0644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, sample := range report.Samples {
		for _, pass := range sample.Metrics {
			if !pass {
				os.Exit(1)
			}
		}
	}
}

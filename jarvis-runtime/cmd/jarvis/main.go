package main

import (
	"flag"
	"fmt"
	"os"

	"jarvis-runtime/internal/orchestrator"
)

func main() {
	var taskText string
	var workspacePath string

	flag.StringVar(&taskText, "task", "", "task text")
	flag.StringVar(&workspacePath, "workspace", "", "path to target workspace repo")
	flag.Parse()

	if taskText == "" || workspacePath == "" {
		fmt.Fprintln(os.Stderr, "usage: jarvis run --task \"...\" --workspace /path/to/repo")
		fmt.Fprintln(os.Stderr, "example: jarvis run --task \"bootstrap workspace + run tests\" --workspace ../car-rental-api")
		os.Exit(2)
	}

	if err := orchestrator.Run(taskText, workspacePath); err != nil {
		fmt.Fprintln(os.Stderr, "FAILED:", err.Error())
		os.Exit(1)
	}

	fmt.Println("OK: task completed successfully")
}

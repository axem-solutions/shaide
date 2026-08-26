package main

import (
	"log"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow"
)

func main() {
	if err := workflow.Run(); err != nil {
		log.Fatalf("installer failed: %v", err)
	}
}

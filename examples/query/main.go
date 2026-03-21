// Example: send a one-off query to Claude.
// Requires ANTHROPIC_API_KEY environment variable.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/i2y/pyffi/casdk"
)

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Fatal("Set ANTHROPIC_API_KEY environment variable")
	}

	client, err := casdk.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	prompt := "What is 2+2? Answer in one sentence."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	fmt.Printf("Prompt: %s\n\n", prompt)

	for msg, err := range client.Query(context.Background(), prompt,
		casdk.WithMaxTurns(1),
		casdk.WithPermissionMode("plan"),
	) {
		if err != nil {
			log.Fatal(err)
		}
		switch msg.Type() {
		case "assistant":
			for _, block := range msg.ContentBlocks() {
				if tb, ok := block.(casdk.TextBlock); ok {
					fmt.Println(tb.Text)
				}
			}
		case "result":
			fmt.Printf("\n--- Result ---\n")
			if msg.TotalCostUSD() > 0 {
				fmt.Printf("Cost: $%.4f\n", msg.TotalCostUSD())
			}
			if msg.NumTurns() > 0 {
				fmt.Printf("Turns: %d\n", msg.NumTurns())
			}
			if u := msg.Usage(); u != nil {
				fmt.Printf("Tokens: %d in / %d out\n", u.InputTokens, u.OutputTokens)
			}
		}
	}
}

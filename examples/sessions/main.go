// Example: list recent Claude Code sessions and show the first prompt of each.
package main

import (
	"fmt"
	"log"

	"github.com/i2y/pyffi/casdk"
)

func main() {
	client, err := casdk.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	sessions, err := client.ListSessions(casdk.WithLimit(5))
	if err != nil {
		log.Fatal(err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	for _, s := range sessions {
		title := s.CustomTitle
		if title == "" {
			title = s.Summary
		}
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("%-40s  %s  %s\n", truncate(title, 40), s.LastModified.Format("2006-01-02 15:04"), s.SessionID[:8])
	}

	// Show messages from the most recent session.
	fmt.Printf("\n--- Messages from session %s ---\n", sessions[0].SessionID[:8])
	msgs, err := client.GetSessionMessages(sessions[0].SessionID, casdk.WithMessageLimit(3))
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range msgs {
		content := truncate(m.Content, 100)
		fmt.Printf("[%s] %s\n", m.Type, content)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

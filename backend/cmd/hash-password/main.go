package main

import (
	"fmt"
	"os"

	"gamegen/backend/internal/platform/security"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/hash-password '<password>'")
		os.Exit(2)
	}

	hash, err := security.NewPasswordHasher().Hash(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(hash)
}

package main

import (
	"fmt"
	"os"

	"github.com/dpassarelli/cherry-sync/internal/cli"
	"github.com/dpassarelli/cherry-sync/internal/compare"
)

func main() {
	a, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: csync SOURCE DESTINATION")
		os.Exit(2)
	}

	fmt.Println("Source:", a.Source)
	fmt.Println("Destination:", a.Destination)

	result, err := compare.Run(a.Source, a.Destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Changes:", len(result.Actions))
	for _, act := range result.Actions {
		fmt.Printf("  %s %s\n", act.Verb, act.Path)
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/dpassarelli/cherry-sync/internal/cli"
)

func main() {
	a, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: csync SOURCE DESTINATION")
		os.Exit(2)
	}
	fmt.Println("Source:", a.Source)
	fmt.Println("Destination:", a.Destination)
}

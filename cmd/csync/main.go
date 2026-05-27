package main

import (
	"fmt"
	"os"

	"github.com/dpassarelli/cherry-sync/internal/cli"
)

func main() {
	a := cli.Parse(os.Args[1:])
	fmt.Println("Source:", a.Source)
	fmt.Println("Destination:", a.Destination)
}

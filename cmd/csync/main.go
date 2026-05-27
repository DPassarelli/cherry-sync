package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Source:", os.Args[1])
	fmt.Println("Destination:", os.Args[2])
}

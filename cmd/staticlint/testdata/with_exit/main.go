package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("Just small test")
	os.Exit(1) // want "use os.Exit"
}

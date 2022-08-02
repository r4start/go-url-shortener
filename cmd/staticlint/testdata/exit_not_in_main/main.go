package main

import (
	"fmt"
	"os"
)

func main() {
	printGreetings()
}

func printGreetings() {
	fmt.Printf("Just small test")
	os.Exit(1)
}

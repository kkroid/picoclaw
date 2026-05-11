package main

import (
	"fmt"
	"os"

	backend "github.com/sipeed/oneappfactory/web/backend"
)

func main() {
	if err := backend.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

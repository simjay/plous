package main

import (
	"fmt"
	"os"

	bench "github.com/simjay/plous/benchmark"
)

func main() {
	if err := bench.Toolkit(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

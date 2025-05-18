package main

import (
	"fmt"

	prime "github.com/gologames/go-lab/prime-numbers-benchmark/internal"
)

func main() {
	nums := prime.CalculatePrimes(100000, 10)
	fmt.Printf("%v", nums)
}

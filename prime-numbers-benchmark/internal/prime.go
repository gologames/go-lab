package prime

import (
	"math"
	"sort"
	"sync"
)

func CalculatePrimes(maxNum int, workerCount int) []int {
	part := maxNum / workerCount
	ch := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := range workerCount {
		from := i * part
		to := from + part
		if i == workerCount-1 {
			to = maxNum
		}
		go worker(from, to, ch, &wg)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	result := []int{}
	for num := range ch {
		result = append(result, num)
	}

	sort.Ints(result)
	return result
}

func worker(from int, to int, ch chan int, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := from; i < to; i++ {
		if isPrime(i) {
			ch <- i
		}
	}
}

func isPrime(num int) bool {
	if num < 2 {
		return false
	}

	sqrt := int(math.Sqrt(float64(num)))

	for i := 2; i <= sqrt; i++ {
		if num%i == 0 {
			return false
		}
	}

	return true
}

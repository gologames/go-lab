package prime

import (
	"testing"
)

const maxNum = 100_000_000

func BenchmarkPrime_1(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 1)
	}
}

func BenchmarkPrime_2(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 2)
	}
}

func BenchmarkPrime_4(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 4)
	}
}

func BenchmarkPrime_8(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 8)
	}
}

func BenchmarkPrime_16(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 16)
	}
}

func BenchmarkPrime_32(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 32)
	}
}

func BenchmarkPrime_64(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 64)
	}
}

func BenchmarkPrime_128(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 128)
	}
}

func BenchmarkPrime_256(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 256)
	}
}

func BenchmarkPrime_512(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 512)
	}
}

func BenchmarkPrime_1024(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 1024)
	}
}

func BenchmarkPrime_1048576(b *testing.B) {
	for b.Loop() {
		CalculatePrimes(maxNum, 1048576)
	}
}

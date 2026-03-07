package main

import "fmt"

// factorial calculates the factorial of a non-negative integer n
// Returns n! = n * (n-1) * (n-2) * ... * 1
// For n = 0, returns 1 (0! = 1 by definition)
func factorial(n int) int {
	if n < 0 {
		return -1 // Invalid input
	}
	if n == 0 || n == 1 {
		return 1
	}
	
	result := 1
	for i := 2; i <= n; i++ {
		result *= i
	}
	return result
}

func main() {
	// Test the factorial function with some examples
	testCases := []int{0, 1, 2, 3, 4, 5, 6, 10}
	
	fmt.Println("Factorial calculations:")
	for _, n := range testCases {
		result := factorial(n)
		fmt.Printf("%d! = %d\n", n, result)
	}
	
	// Test with invalid input
	fmt.Printf("\nInvalid input test:")
	fmt.Printf("\n(-1)! = %d (should return -1 for negative input)\n", factorial(-1))
}
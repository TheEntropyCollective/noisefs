// Demo command - demonstrates NoiseFS core functionality
package main

import (
	"flag"
	"fmt"
)

func main() {
	var (
		reuseDemo = flag.Bool("reuse", false, "Run the block reuse demonstration")
	)
	flag.Parse()

	if *reuseDemo {
		fmt.Println("Block reuse demonstration")
		fmt.Println("To run the demo, use: go run cmd/noisefs/demo.go")
	} else {
		fmt.Println("NoiseFS core functionality demonstration")
		fmt.Println("To run the demo, use: go run cmd/noisefs/demo.go")
	}
}
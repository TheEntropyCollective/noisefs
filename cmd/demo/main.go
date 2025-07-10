// Demo command - demonstrates NoiseFS core functionality
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		reuseDemo = flag.Bool("reuse", false, "Run the block reuse demonstration")
	)
	flag.Parse()

	var err error
	if *reuseDemo {
		err = runDemoReuse()
	} else {
		err = runDemo()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Demo failed: %v\n", err)
		os.Exit(1)
	}
}

// Include the demo functions from the other file
// In a real implementation, these would be in a shared package
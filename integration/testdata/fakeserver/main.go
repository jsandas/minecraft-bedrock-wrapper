// Command fakeserver provides a controllable stand-in for the Bedrock process.
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "ready":
			fmt.Println("Server started")
		case "say hello":
			fmt.Println("stdout: say hello")
			fmt.Fprintln(os.Stderr, "stderr: say hello")
		case "stop":
			return
		case "fail":
			os.Exit(7)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

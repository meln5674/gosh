// Used as part of gosh test suite, reads a single byte from stdin, prints it to stdout
package main

import "os"

func main() {
	c := make([]byte, 1)
	if _, err := os.Stdin.Read(c); err != nil {
		panic(err)
	}
	if _, err := os.Stdout.Write(c); err != nil {
		panic(err)
	}
}

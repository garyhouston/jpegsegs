package main

import (
	"bufio"
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	"log"
	"os"
)

// Print the JPEG markers and segment lengths, up to SOS.
func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s file\n", os.Args[0])
		return
	}
	in, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()
	reader := bufio.NewReader(in)
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		log.Fatal(err)
	}
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			log.Fatal(err)
		}
		if marker == jseg.SOS {
			fmt.Printf("%s, scan data follows\n", marker.Name())
			break
		}
		fmt.Printf("%s, %d bytes\n", marker.Name(), len(buf))
	}
}

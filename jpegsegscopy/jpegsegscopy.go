package main

import (
	"bufio"
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	"log"
	"os"
)

// Copy a JPEG file one segment at a time without modifying it.
func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Usage: %s infile outfile\n", os.Args[0])
		return
	}
	in, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()
	reader := bufio.NewReader(in)
	scanner, segments, err := jseg.ReadAll(reader)
	if err != nil {
		log.Fatal(err)
	}
	out, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	writer := bufio.NewWriter(out)
	dumper, err := jseg.WriteAll(writer, segments);
	if err != nil {
		log.Fatal(err)
	}
	if err := dumper.Copy(scanner); err != nil {
		log.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		log.Fatal(err)
	}
}

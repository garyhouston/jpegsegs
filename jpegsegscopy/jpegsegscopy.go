package main

import (
	"fmt"
	"io"
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
	reader, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		log.Fatal(err)
	}
	writer, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer writer.Close()
	dumper, err := jseg.NewDumper(writer)
	if err != nil {
		log.Fatal(err)
	}
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			log.Fatal(err)
		}
		if err := dumper.Dump(marker, buf); err != nil {
			log.Fatal(err)
		}
		if marker == jseg.EOI {
			break
		}
		
	}
	// There may be more images after the EOI marker if the file is
	// using Multi-Picture Format. Just copy it for now.
	if _, err := io.Copy(writer, reader); err != nil {
		log.Fatal(err)
	}
}

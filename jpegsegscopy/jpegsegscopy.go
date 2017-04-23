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
	dumper, err := jseg.WriteAll(writer, segments)
	if err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, 10000)
	for {
		var marker jseg.Marker
		buf, marker, err = jseg.ReadImageData(reader, buf)
		if err != nil {
			log.Fatal(err)
		}
		err = jseg.WriteImageData(writer, buf)
		if err != nil {
			log.Fatal(err)
		}
		err = jseg.WriteMarker(writer, marker, buf)
		if err != nil {
			log.Fatal(err)
		}
		if marker == jseg.EOI {
			break
		}

	}
	// There may be more images after the EOI marker if the file is
	// using Multi-Picture Format. Just copy it for now.
	if err := dumper.Copy(scanner); err != nil {
		log.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		log.Fatal(err)
	}
}

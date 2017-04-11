package main

import (
	"bufio"
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	"log"
	"os"
)

// Make a copy of a JPEG file with all COM, APP and JPG segments removed.
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
	out, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)		
	}
	defer out.Close()
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		log.Fatal(err)
	}
	dumper, err := jseg.NewDumper(writer)
	if err != nil {
		log.Fatal(err)
	}
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			log.Fatal(err)
		}
		if marker == jseg.COM || marker >= jseg.APP0 && marker <= jseg.APP0+0xf || marker >= jseg.JPG0 && marker <= jseg.JPG0+0xD {
			continue
		}
		if err := dumper.Dump(marker, buf); err != nil {
			log.Fatal(err)
		}
		if marker == jseg.SOS {
			break
		}
	}
	if err := dumper.Copy(scanner); err != nil {
		log.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		log.Fatal(err)
	}

}

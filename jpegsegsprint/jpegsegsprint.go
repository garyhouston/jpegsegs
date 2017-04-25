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
			fmt.Println(marker.Name())
			break
		}
		if marker == jseg.APP0+2 {
			isMPF, _ := jseg.GetMPFHeader(buf)
			if isMPF {
				fmt.Printf("%s, %d bytes (MPF segment)\n", marker.Name(), len(buf))
				continue
			}
		}
		fmt.Printf("%s, %d bytes\n", marker.Name(), len(buf))
	}
	buf := make([]byte, 10000)
	reset := 0
	total := 0
	for {
		var marker jseg.Marker
		var err error
		buf, marker, err = jseg.ReadImageData(reader, buf)
		if err != nil {
			log.Fatal(err)
		}
		total += len(buf)
		if marker >= jseg.RST0 && marker <= jseg.RST0+7 {
			reset++
		} else if marker == jseg.EOI {
			fmt.Printf("%d bytes of scan data and %d reset markers\n", total, reset)
			fmt.Println(marker.Name())
			break
		} else {
			fmt.Printf("%s, unexpected marker\n", marker.Name())
		}
	}
}

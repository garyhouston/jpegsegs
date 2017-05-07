package main

import (
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	"log"
	"os"
)

// Print the JPEG markers and segment lengths.
func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s file\n", os.Args[0])
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
	dataCount := uint32(0)
	resetCount := uint32(0)
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			log.Fatal(err)
		}
		if marker == 0 {
			dataCount += uint32(len(buf))
			continue
		}
		if buf == nil && (marker >= jseg.RST0 && marker <= jseg.RST0+7) {
				resetCount++
				continue
		}
		if dataCount > 0 || resetCount > 0 {
			fmt.Printf("%d bytes of image data", dataCount)
			if resetCount > 0 {
				fmt.Printf(" and %d reset markers", resetCount)
			}
			fmt.Println()
		}
		if buf == nil {
			fmt.Println(marker.Name())
			if marker == jseg.EOI {
				break
			}
			continue
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
}

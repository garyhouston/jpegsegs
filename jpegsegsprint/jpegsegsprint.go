package main

// Print JPEG markers and segment lengths.

import (
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	"io"
	"log"
	"os"
)

// Scan and print a single image, processing any MPF segment found.
func scanImage(reader io.ReadSeeker, mpfProcessor jseg.MPFProcessor) error {
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		return err
	}
	fmt.Println("SOI")
	dataCount := uint32(0)
	resetCount := uint32(0)
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return err
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
			dataCount = 0
			resetCount = 0
		}
		if buf == nil {
			fmt.Println(marker.Name())
			if marker == jseg.EOI {
				return nil
			}
			continue
		}
		if marker == jseg.APP0+2 {
			done, buf, err := mpfProcessor.ProcessAPP2(nil, reader, buf)
			if err != nil {
				return err
			}
			if done {
				fmt.Printf("%s, %d bytes (MPF segment)\n", marker.Name(), len(buf))
				continue
			}
		}
		fmt.Printf("%s, %d bytes\n", marker.Name(), len(buf))
	}
}

// State for the MPF image iterator.
type scanData struct {
}

// Function to be applied to each MPF image: prints image details.
func (scan *scanData) MPFApply(reader io.ReadSeeker, index uint32, length uint32) error {
	if index > 0 {
		return scanImage(reader, &jseg.MPFCheck{})
	}
	return nil
}

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
	var index jseg.MPFGetIndex
	err = scanImage(reader, &index)
	if err != nil {
		log.Fatal(err)
	}
	if index.Index != nil {
		err = index.Index.ImageIterate(reader, &scanData{})
		if err != nil {
			log.Fatal(err)
		}
	}
}

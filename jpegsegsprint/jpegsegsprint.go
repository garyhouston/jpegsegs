package main

// Print JPEG markers and segment lengths.

import (
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	tiff "github.com/garyhouston/tiff66"
	"io"
	"log"
	"os"
)

// MPF processor that reads the index data.
type MPFIndexData struct {
	Index *jseg.MPFIndex // MPF Index info.
}

func (mpfData *MPFIndexData) ProcessAPP2(reader io.Seeker, buf []byte) (bool, error) {
	done := false
	isMPF, next := jseg.GetMPFHeader(buf)
	if isMPF {
		tree, err := jseg.GetMPFTree(buf[next:], tiff.MPFIndexSpace)
		if err != nil {
			return false, err
		}
		// MPF offsets are relative to the start of the MPF
		// header, which is 4 bytes past the start of buf.
		// The current position of the reader is one byte past
		// the data read into buf.
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, err
		}
		offset := uint32(pos) - uint32(len(buf)-4)
		if mpfData.Index, err = jseg.MPFIndexFromTIFF(tree, offset); err != nil {
			return false, err
		}
		done = true
	}
	return done, nil
}

// MPF processor that just records the presence of an MPF segment.
type MPFCheck struct {
}

func (mpfData *MPFCheck) ProcessAPP2(reader io.Seeker, buf []byte) (bool, error) {
	isMPF, _ := jseg.GetMPFHeader(buf)
	return isMPF, nil
}

type MPFProcessor interface {
	ProcessAPP2(reader io.Seeker, buf []byte) (bool, error)
}

// Scan and print a single image, processing any MPF segment found.
func scanImage(reader io.ReadSeeker, mpfProcessor MPFProcessor) error {
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
		}
		if buf == nil {
			fmt.Println(marker.Name())
			if marker == jseg.EOI {
				return nil
			}
			continue
		}
		if marker == jseg.APP0+2 {
			done, err := mpfProcessor.ProcessAPP2(reader, buf)
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
		return scanImage(reader, &MPFCheck{})
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
	var indexData MPFIndexData
	err = scanImage(reader, &indexData)
	if err != nil {
		log.Fatal(err)
	}
	if indexData.Index != nil {
		err = indexData.Index.ImageIterate(reader, &scanData{})
		if err != nil {
			log.Fatal(err)
		}
	}
}

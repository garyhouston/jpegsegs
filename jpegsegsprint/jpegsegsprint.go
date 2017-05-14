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

// Unpack MPF from TIFF. If there's an index of images with offsets,
// convert the offsets to file positions and return them.
func extractOffsets(reader io.Seeker, buf []byte, tiffOffset uint32, mpfSpace tiff.TagSpace) ([]uint32, error) {
	mpfTree, err := jseg.GetMPFTree(buf[tiffOffset:], mpfSpace)
	if err != nil {
		return nil, err
	}
	if mpfTree.Space == tiff.MPFIndexSpace {
		// MPF offsets are relative to the start of the MPF
		// header, which is 4 bytes past the start of buf.
		// The current position of the reader is one byte past
		// the data read into buf.
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		mpfOffset := uint32(pos) - uint32(len(buf)-4)
		return jseg.MPFImageOffsets(mpfTree, mpfOffset)
	}
	return nil, nil
}

// Process a single image. A file using the MPF extension can contain
// multiple images: returns the image offsets relative to the start of
// the file if found.
func scanImage(reader io.ReadSeeker, mpfSpace tiff.TagSpace) ([]uint32, error) {
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		return nil, err
	}
	fmt.Println("SOI")
	dataCount := uint32(0)
	resetCount := uint32(0)
	var offsets []uint32
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return nil, err
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
				return offsets, nil
			}
			continue
		}
		if marker == jseg.APP0+2 {
			isMPF, next := jseg.GetMPFHeader(buf)
			if isMPF {
				fmt.Printf("%s, %d bytes (MPF segment)\n", marker.Name(), len(buf))
				offsets, err = extractOffsets(reader, buf, next, mpfSpace)
				if err != nil {
					return nil, err
				}
				continue
			}
		}
		fmt.Printf("%s, %d bytes\n", marker.Name(), len(buf))
	}
}

func scanMPFImages(reader io.ReadSeeker, offsets []uint32) error {
	count := uint32(len(offsets))
	for i := uint32(0); i < count; i++ {
		if offsets[i] > 0 {
			fmt.Printf("MPF image at offset %d\n", offsets[i])
			if _, err := reader.Seek(int64(offsets[i]), io.SeekStart); err != nil {
				return err
			}
			if _, err := scanImage(reader, tiff.MPFAttributeSpace); err != nil {
				return err
			}
		}
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
	offsets, err := scanImage(reader, tiff.MPFIndexSpace)
	if err != nil {
		log.Fatal(err)
	}
	if len(offsets) > 0 {
		err = scanMPFImages(reader, offsets)
		if err != nil {
			log.Fatal(err)
		}
	}
}

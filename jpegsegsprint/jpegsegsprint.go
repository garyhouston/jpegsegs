package main

// Print JPEG markers and segment lengths.

import (
	"errors"
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	tiff "github.com/garyhouston/tiff66"
	"io"
	"log"
	"os"
)

// Process a single image. A file using the MPF extensions can contain
// multiple images. Returns the MPF segment and MPF offset, if found.
func scanImage(reader io.ReadSeeker) ([]byte, uint32, error) {
	var mpfSegment []byte
	mpfOffset := uint32(0)
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		return nil, 0, err
	}
	fmt.Println("SOI")
	dataCount := uint32(0)
	resetCount := uint32(0)
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return nil, 0, err
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
				return mpfSegment, mpfOffset, nil
			}
			continue
		}
		if marker == jseg.APP0+2 {
			isMPF, next := jseg.GetMPFHeader(buf)
			if isMPF {
				fmt.Printf("%s, %d bytes (MPF segment)\n", marker.Name(), len(buf))
				mpfSegment = make([]byte, len(buf[next:]))
				copy(mpfSegment, buf[next:])
				// MPF offsets are relative to the start of the
				// MPF header, which is 4 bytes past the start
				// of buf.
				pos, err := reader.Seek(0, io.SeekCurrent)
				if err != nil {
					return nil, 0, err
				}
				mpfOffset = uint32(pos) - uint32(len(buf)-4)
				continue
			}
		}
		fmt.Printf("%s, %d bytes\n", marker.Name(), len(buf))
	}
}

func scanMPFImages(reader io.ReadSeeker, mpfSegment []byte, mpfOffset uint32) error {
	if mpfSegment != nil {
		mpfTree, err := jseg.GetMPFTree(mpfSegment)
		if err != nil {
			return err
		}
		if mpfTree.Space != tiff.MPFIndexSpace {
			return errors.New("MPF segment doesn't contain Index")
		}
		order := mpfTree.Order
		count := uint32(0)
		sizes := []uint32(nil)
		offsets := []uint32(nil)
		for _, f := range mpfTree.Fields {
			switch f.Tag {
			case jseg.MPFNumberOfImages:
				count = f.Long(0, order)
				sizes = make([]uint32, count)
				offsets = make([]uint32, count)
			case jseg.MPFEntry:
				for i := uint32(0); i < count; i++ {
					sizes[i] = f.Long(i*4+1, order)
					offsets[i] = f.Long(i*4+2, order)
				}
			}
		}
		for i := uint32(0); i < count; i++ {
			if offsets[i] > 0 {
				fmt.Printf("MPF image %d at offset %d, size %d\n", i+1, mpfOffset+offsets[i], sizes[i])
				if _, err = reader.Seek(int64(offsets[i]+mpfOffset), io.SeekStart); err != nil {
					return err
				}
				if _, _, err := scanImage(reader); err != nil {
					return err
				}
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
	mpfSegment, mpfOffset, err := scanImage(reader)
	if err != nil {
		log.Fatal(err)
	}
	err = scanMPFImages(reader, mpfSegment, mpfOffset)
	if err != nil {
		log.Fatal(err)
	}
}

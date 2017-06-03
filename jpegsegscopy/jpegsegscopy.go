package main

// Unpack a JPEG file one segment at a time and repackage into a new
// JPEG file.  It can process files which use the Multi-Picture Format
// extension to contain multiple images.

import (
	"fmt"
	jseg "github.com/garyhouston/jpegsegs"
	tiff "github.com/garyhouston/tiff66"
	"io"
	"log"
	"os"
)

// MPFAttributeData conforms to the jseg.MPFProcessor interface. It's
// applied to the APP2 segment of images after the first, to decode
// and reencode MPF attribute data (not for any particular reason, but
// to show it can be done.)
type MPFAttributeData struct {
}

func (mpfData *MPFAttributeData) ProcessAPP2(writer io.WriteSeeker, reader io.ReadSeeker, buf []byte) (bool, []byte, error) {
	isMPF, next := jseg.GetMPFHeader(buf)
	if isMPF {
		tree, err := jseg.GetMPFTree(buf[next:], tiff.MPFAttributeSpace)
		if err != nil {
			return false, nil, err
		}
		tree.Fix()
		buf, err = jseg.MakeMPFSegment(tree)
		if err != nil {
			return false, nil, err
		}
	}
	return isMPF, buf, nil
}

// Copy a single image, processing any MPF segment found.
func copyImage(writer io.WriteSeeker, reader io.ReadSeeker, mpfProcessor jseg.MPFProcessor) error {
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		return err
	}
	dumper, err := jseg.NewDumper(writer)
	if err != nil {
		return err
	}
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return err
		}
		if marker == jseg.APP0+2 {
			_, buf, err = mpfProcessor.ProcessAPP2(writer, reader, buf)
			if err != nil {
				return err
			}
		}
		if err := dumper.Dump(marker, buf); err != nil {
			return err
		}
		if marker == jseg.EOI {
			break
		}
	}
	return nil
}

// State for MPF image iterator.
type copyData struct {
	writer     io.WriteSeeker
	newOffsets []uint32
}

// Function to be applied to each MPF image: copies the image to the
// output stream.
func (copy *copyData) MPFApply(reader io.ReadSeeker, index uint32, length uint32) error {
	if index > 0 {
		pos, err := copy.writer.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		copy.newOffsets[index] = uint32(pos)
		var mpfAttribute MPFAttributeData
		return copyImage(copy.writer, reader, &mpfAttribute)
	}
	return nil
}

// Copy additional images specified with MPF.
func copyMPFImages(writer io.WriteSeeker, reader io.ReadSeeker, index *jseg.MPFIndex) ([]uint32, error) {
	var copy copyData
	copy.writer = writer
	copy.newOffsets = make([]uint32, len(index.ImageOffsets))
	index.ImageIterate(reader, &copy)
	return copy.newOffsets, nil
}

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
	writer, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer writer.Close()
	var mpfIndex jseg.MPFIndexRewriter
	if err = copyImage(writer, reader, &mpfIndex); err != nil {
		log.Fatal(err)
	}
	if mpfIndex.Tree != nil {
		newOffsets, err := copyMPFImages(writer, reader, mpfIndex.Index)
		if err != nil {
			log.Fatal(err)
		}
		end, err := writer.Seek(0, io.SeekCurrent)
		if err != nil {
			log.Fatal(err)
		}
		if err = jseg.RewriteMPF(writer, mpfIndex.Tree, mpfIndex.APP2WritePos, newOffsets, uint32(end)); err != nil {
			log.Fatal(err)
		}
	}
}

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

// MPF processor that reads and repacks the index data.
type MPFIndexData struct {
	Tree         *tiff.IFDNode // Unpacked MPF index TIFF tree.
	MPFOffset    uint32        // Offset of byte following MPF header in input stream.
	ImageOffsets []uint32      // Offsets of images in input stream, relative to MPFOffset, or 0 for the first image.
	APP2WritePos uint32        // Position of the MPF APP2 marker in the output stream.
}

func (mpfData *MPFIndexData) ProcessAPP2(writer io.Seeker, reader io.Seeker, buf []byte) (bool, []byte, error) {
	done := false
	isMPF, next := jseg.GetMPFHeader(buf)
	if isMPF {
		savebuf := make([]byte, len(buf)-jseg.MPFHeaderSize)
		copy(savebuf, buf[next:])
		var err error
		mpfData.Tree, err = jseg.GetMPFTree(savebuf, tiff.MPFIndexSpace)
		if err != nil {
			return false, nil, err
		}
		mpfData.Tree.Fix()
		// MPF offsets are relative to the byte following the
		// MPF header, which is 4 bytes past the start of buf.
		// The current position of the reader is one byte past
		// the data read into buf.
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, nil, err
		}
		mpfData.MPFOffset = uint32(pos) - uint32(len(buf)-4)
		mpfData.ImageOffsets, err = jseg.MPFImageOffsets(mpfData.Tree, mpfData.MPFOffset)
		if err != nil {
			return false, nil, err
		}
		buf, err = jseg.MakeMPFSegment(mpfData.Tree)
		if err != nil {
			return false, nil, err
		}
		pos, err = writer.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, nil, err
		}
		mpfData.APP2WritePos = uint32(pos)
		done = true
	}
	return done, buf, nil
}

// MPF processor that reads and repacks the attribute data.
type MPFAttributeData struct {
	Tree *tiff.IFDNode // Unpacked MPF attribute TIFF tree.
}

func (mpfData *MPFAttributeData) ProcessAPP2(writer io.Seeker, reader io.Seeker, buf []byte) (bool, []byte, error) {
	done := false
	isMPF, next := jseg.GetMPFHeader(buf)
	if isMPF {
		savebuf := make([]byte, len(buf)-jseg.MPFHeaderSize)
		copy(savebuf, buf[next:])
		var err error
		mpfData.Tree, err = jseg.GetMPFTree(savebuf, tiff.MPFAttributeSpace)
		if err != nil {
			return false, nil, err
		}
		mpfData.Tree.Fix()
		buf, err = jseg.MakeMPFSegment(mpfData.Tree)
		if err != nil {
			return false, nil, err
		}
		done = true
	}
	return done, buf, nil
}

// MPF processor that does nothing.
type MPFDummyData struct {
}

func (mpfData *MPFDummyData) ProcessAPP2(writer io.Seeker, reader io.Seeker, buf []byte) (bool, []byte, error) {
	return false, buf, nil
}

type MPFProcessor interface {
	ProcessAPP2(writer io.Seeker, reader io.Seeker, buf []byte) (bool, []byte, error)
}

// Copy a single image, processing any MPF segment found.
func copyImage(writer io.WriteSeeker, reader io.ReadSeeker, mpfProcessor MPFProcessor) error {
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

// Copy additional images specified with MPF.
func copyMPFImages(writer io.WriteSeeker, reader io.ReadSeeker, offsets []uint32) ([]uint32, error) {
	count := uint32(len(offsets))
	newOffsets := make([]uint32, len(offsets))
	for i := uint32(0); i < count; i++ {
		if offsets[i] > 0 {
			if _, err := reader.Seek(int64(offsets[i]), io.SeekStart); err != nil {
				return nil, err
			}
			pos, err := writer.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, err
			}
			newOffsets[i] = uint32(pos)
			// Processing the MPF attribute data could be omitted,
			// by passing a MPFDummyData object.
			var mpfAttribute MPFAttributeData
			if err := copyImage(writer, reader, &mpfAttribute); err != nil {
				return nil, err
			}
		}
	}
	return newOffsets, nil
}

// Modify a MPF TIFF tree with new image offsets and sizes, then overwrite the
// MPF data in the output stream.
func rewriteMPF(writer io.WriteSeeker, mpfTree *tiff.IFDNode, mpfWritePos uint32, offsets, lengths []uint32) error {
	jseg.SetMPFImagePositions(mpfTree, mpfWritePos+8, offsets, lengths)
	newbuf, err := jseg.MakeMPFSegment(mpfTree)
	if err != nil {
		return err
	}
	if _, err := writer.Seek(int64(mpfWritePos), io.SeekStart); err != nil {
		return err
	}
	if err := jseg.WriteMarker(writer, jseg.APP0+2); err != nil {
		return err
	}
	if err := jseg.WriteData(writer, newbuf); err != nil {
		return err
	}
	return nil
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
	var mpfIndex MPFIndexData
	if err = copyImage(writer, reader, &mpfIndex); err != nil {
		log.Fatal(err)
	}
	if mpfIndex.Tree != nil {
		newOffsets, err := copyMPFImages(writer, reader, mpfIndex.ImageOffsets)
		if err != nil {
			log.Fatal(err)
		}
		numImages := len(newOffsets)
		newLengths := make([]uint32, numImages)
		for i := 0; i < numImages-1; i++ {
			newLengths[i] = newOffsets[i+1] - newOffsets[i]
		}
		lastpos, err := writer.Seek(0, io.SeekCurrent)
		if err != nil {
			log.Fatal(err)
		}
		newLengths[numImages-1] = uint32(lastpos) - newOffsets[numImages-1]
		if err = rewriteMPF(writer, mpfIndex.Tree, mpfIndex.APP2WritePos, newOffsets, newLengths); err != nil {
			log.Fatal(err)
		}
	}
}

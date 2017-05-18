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

// Copy a single image. If a MPF segment is found in APP2, return a
// MPF TIFF tree, the MPF image offsets in the input file, and the
// position where the MPF segment was written in the output file.
func copyImage(writer io.WriteSeeker, reader io.ReadSeeker, mpfSpace tiff.TagSpace) (*tiff.IFDNode, []uint32, uint32, error) {
	scanner, err := jseg.NewScanner(reader)
	if err != nil {
		return nil, nil, 0, err
	}
	dumper, err := jseg.NewDumper(writer)
	if err != nil {
		return nil, nil, 0, err
	}
	var mpfTree *tiff.IFDNode
	var offsets []uint32
	var mpfWritePos uint32
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return nil, nil, 0, err
		}
		if marker == jseg.APP0+2 {
			isMPF, next := jseg.GetMPFHeader(buf)
			if isMPF {
				// Copy buf so that it can be used outside this loop via pointers in the TIFF tree.
				savebuf := make([]byte, len(buf)-jseg.MPFHeaderSize)
				copy(savebuf, buf[next:])
				mpfTree, err = jseg.GetMPFTree(savebuf, mpfSpace)
				if err != nil {
					return nil, nil, 0, err
				}
				mpfTree.Fix()
				if mpfTree.Space == tiff.MPFIndexSpace {
					// MPF offsets are relative to
					// the byte following the MPF
					// header, which is 4 bytes
					// past the start of buf.  The
					// current position of the
					// reader is one byte past the
					// data read into buf.
					pos, err := reader.Seek(0, io.SeekCurrent)
					if err != nil {
						return nil, nil, 0, err
					}
					mpfOffset := uint32(pos) - uint32(len(buf)-4)
					offsets, err = jseg.MPFImageOffsets(mpfTree, mpfOffset)
					if err != nil {
						return nil, nil, 0, err
					}
				}
				buf, err = jseg.MakeMPFSegment(mpfTree)
				if err != nil {
					return nil, nil, 0, err
				}
				pos, err := writer.Seek(0, io.SeekCurrent)
				if err != nil {
					return nil, nil, 0, err
				}
				mpfWritePos = uint32(pos)
			}
		}
		if err := dumper.Dump(marker, buf); err != nil {
			return nil, nil, 0, err
		}
		if marker == jseg.EOI {
			break
		}
	}
	return mpfTree, offsets, mpfWritePos, nil
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
			if _, _, _, err := copyImage(writer, reader, tiff.MPFAttributeSpace); err != nil {
				return nil, err
			}
			pos, err = writer.Seek(0, io.SeekCurrent)
			if err != nil {
				return nil, err
			}
		}
	}
	return newOffsets, nil
}

// Modify a MPF TIFF tree with new image offsets and sizes, then overwrite the
// MPF data in the output stream.
func rewriteMPF(writer io.WriteSeeker, mpfTree *tiff.IFDNode, mpfWritePos uint32, offsets, lengths []uint32) error {
	jseg.SetMPFImagePositions(mpfTree, mpfWritePos + 8, offsets, lengths)
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
	mpfTree, imageOffsets, mpfWritePos, err := copyImage(writer, reader, tiff.MPFIndexSpace)
	if err != nil {
		log.Fatal(err)
	}
	if mpfTree != nil && mpfTree.Space == tiff.MPFIndexSpace {
		newOffsets, err := copyMPFImages(writer, reader, imageOffsets)
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
		if err = rewriteMPF(writer, mpfTree, mpfWritePos, newOffsets, newLengths); err != nil {
			log.Fatal(err)
		}

	}
}

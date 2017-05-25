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
	Tree         *tiff.IFDNode  // Unpacked MPF index TIFF tree.
	Index        *jseg.MPFIndex // MPF Index info.
	APP2WritePos uint32         // Position of the MPF APP2 marker in the output stream.
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
		offset := uint32(pos) - uint32(len(buf)-4)
		if mpfData.Index, err = jseg.MPFIndexFromTIFF(mpfData.Tree, offset); err != nil {
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
}

func (mpfData *MPFAttributeData) ProcessAPP2(writer io.Seeker, reader io.Seeker, buf []byte) (bool, []byte, error) {
	done := false
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
		// Processing the MPF attribute data could be omitted,
		// by passing a MPFDummyData object.
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

// Modify a MPF Tiff tree with new image offsets and sizes, given the
// offsets and the end of file position.
func setMPFImagePositions(mpfTree *tiff.IFDNode, mpfOffset uint32, offsets []uint32, end uint32) {
	count := len(offsets)
	lengths := make([]uint32, count)
	for i := 0; i < count-1; i++ {
		lengths[i] = offsets[i+1] - offsets[i]
	}
	lengths[count-1] = end - offsets[count-1]
	indexWrite := jseg.MPFIndex{mpfOffset, offsets, lengths}
	indexWrite.PutToTiff(mpfTree)
}

// Modify a MPF TIFF tree with new image offsets and sizes, then overwrite the
// MPF data in the output stream.
func rewriteMPF(writer io.WriteSeeker, mpfTree *tiff.IFDNode, mpfWritePos uint32, offsets []uint32) error {
	end, err := writer.Seek(0, io.SeekCurrent)
	if err != nil {
		log.Fatal(err)
	}
	setMPFImagePositions(mpfTree, mpfWritePos+8, offsets, uint32(end))
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
		newOffsets, err := copyMPFImages(writer, reader, mpfIndex.Index)
		if err != nil {
			log.Fatal(err)
		}
		if err = rewriteMPF(writer, mpfIndex.Tree, mpfIndex.APP2WritePos, newOffsets); err != nil {
			log.Fatal(err)
		}
	}
}

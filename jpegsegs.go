package jpegsegs

import (
	"bytes"
	"errors"
	"fmt"
	tiff "github.com/garyhouston/tiff66"
	"io"
)

const (
	TEM  = 0x01
	SOF0 = 0xC0 // SOFn = SOF0+n, n = 0-15 excluding 4, 8 and 12
	DHT  = 0xC4
	JPG  = 0xC8
	DAC  = 0xCC
	RST0 = 0xD0 // RSTn = RST0+n, n = 0-7
	SOI  = 0xD8
	EOI  = 0xD9
	SOS  = 0xDA
	DQT  = 0xDB
	DNL  = 0xDC
	DRI  = 0xDD
	DHP  = 0xDE
	EXP  = 0xDF
	APP0 = 0xE0 // APPn = APP0+n, n = 0-15
	JPG0 = 0xF0 // JPGn = JPG0+n  n = 0-13
	COM  = 0xFE
)

// Marker represents a JPEG marker, which usually indicates the start of a
// segment.
type Marker uint8

var markerNames [256]string

// Initialize markerNames
func init() {
	markerNames[0] = "NUL"
	markerNames[TEM] = "TEM"
	markerNames[DHT] = "DHT"
	markerNames[JPG] = "JPG"
	markerNames[DAC] = "DAC"
	markerNames[SOI] = "SOI"
	markerNames[EOI] = "EOI"
	markerNames[SOS] = "SOS"
	markerNames[DQT] = "DQT"
	markerNames[DNL] = "DNL"
	markerNames[DRI] = "DRI"
	markerNames[DHP] = "DHP"
	markerNames[EXP] = "EXP"
	markerNames[COM] = "COM"
	markerNames[0xFF] = "FILL"

	var i Marker
	for i = 0x02; i <= 0xBF; i++ {
		markerNames[i] = fmt.Sprintf("RES%.2X", i) // Reserved
	}
	for i = SOF0; i <= SOF0+0xF; i++ {
		if i == SOF0+4 || i == SOF0+8 || i == SOF0+12 {
			continue
		}
		markerNames[i] = fmt.Sprintf("SOF%d", i-SOF0)
	}
	for i = RST0; i <= RST0+7; i++ {
		markerNames[i] = fmt.Sprintf("RST%d", i-RST0)
	}
	for i = APP0; i <= APP0+0xF; i++ {
		markerNames[i] = fmt.Sprintf("APP%d", i-APP0)
	}
	for i = JPG0; i <= JPG0+0xD; i++ {
		markerNames[i] = fmt.Sprintf("JPG%d", i-JPG0)
	}
}

// Name returns the name of a marker value.
func (m Marker) Name() string {
	return markerNames[m]
}

// HeaderSize is the size of a JPEG file header.
const HeaderSize = 2

// IsJPEGHeader indicates if buffer contains a JPEG header.
func IsJPEGHeader(buf []byte) bool {
	return buf[0] == 0xFF && buf[1] == SOI
}

// ReadHeader reads the JPEG header (SOI marker). Filler bytes aren't allowed.
func ReadHeader(reader io.Reader, buf []byte) error {
	buf = buf[0:2]
	if _, err := io.ReadFull(reader, buf); err != nil {
		return err
	}
	if !IsJPEGHeader(buf) {
		return errors.New("SOI marker not found")
	}
	return nil
}

// ReadMarker reads a JPEG marker: a pair of bytes starting with 0xFF.
func ReadMarker(reader io.Reader, buf []byte) (Marker, error) {
	buf = buf[0:2]
	if _, err := io.ReadFull(reader, buf); err != nil {
		return 0, err
	}
	if buf[0] != 0xFF {
		return 0, errors.New("0xFF expected in marker")
	}
	buf = buf[1:2] // Look at the 2nd byte only.
	for {
		// Skip 0xFF fill bytes. Fill bytes don't seem to have
		// any purpose, so can be discarded.
		if buf[0] != 0xFF {
			break
		}
		if _, err := reader.Read(buf); err != nil {
			return 0, err
		}
	}
	if buf[0] == 0 {
		return 0, errors.New("Invalid marker 0")
	}
	return Marker(buf[0]), nil
}

// WriteMarker writes a JPEG marker: a pair of bytes starting with 0xFF.
func WriteMarker(writer io.Writer, marker Marker) error {
	buf := make([]byte, 2)
	buf[0] = 0xFF
	buf[1] = byte(marker)
	_, err := writer.Write(buf)
	return err
}

// ReadData writes a JPEG data segment, which follows a marker.
func ReadData(reader io.Reader, buf []byte) ([]byte, error) {
	buf = buf[0:2]
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}
	length := int(buf[0])<<8 + int(buf[1]) - 2
	buf = buf[0:length]
	_, err := io.ReadFull(reader, buf)
	return buf, err
}

// WriteData writes a JPEG data segment, which follows a marker.
func WriteData(writer io.Writer, buf []byte) error {
	len := len(buf) + 2
	if len >= 2<<15 {
		return errors.New(fmt.Sprintf("writeData: data is too long (%d), max 2^16 - 3 (%d)", len-2, 2<<15-3))
	}
	lenbuf := make([]byte, 2)
	lenbuf[0] = byte(len / 256)
	lenbuf[1] = byte(len % 256)
	if _, err := writer.Write(lenbuf); err != nil {
		return err
	}
	_, err := writer.Write(buf)
	return err
}

// Check that a buffer is large enough and reallocate if needed.
func checkbuf(buf []byte, reqsize uint32) []byte {
	current := uint32(cap(buf))
	if current < reqsize {
		newsize := current * 2
		if newsize < reqsize {
			newsize = reqsize
		}
		newbuf := make([]byte, newsize)
		copy(newbuf, buf)
		buf = newbuf
	}
	return buf
}

// ReadImageData reads image scan data up to the next marker. 'buf' is
// either a buffer to read into, which will be reallocated if
// required, or nil to allocate a new buffer. Returns a buffer with
// the image data.
func ReadImageData(reader io.ReadSeeker, buf []byte) ([]byte, error) {
	// Image data could be very large. Reading one byte at a time
	// would be slow. Can't take a buffered reader as a paramater,
	// since two bytes of undo are needed to drop the marker that
	// terminates the segment.
	blocksize := uint32(10000)
	if buf == nil {
		buf = make([]byte, blocksize)
	} else {
		buf = buf[:cap(buf)]
	}
	bufpos := uint32(0)
	readpos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	skipped := uint32(0)
NEXTBLOCK:
	for {
		if bufpos+blocksize < bufpos {
			return nil, errors.New("Read ~4GB image data without finding a terminating marker")
		}
		buf = checkbuf(buf, bufpos+blocksize)
		count, err := reader.Read(buf[bufpos : bufpos+blocksize])
		if err != nil {
			return nil, err
		}
		end := bufpos + uint32(count)
	NEXTINDEX:
		for {
			ffpos := bytes.IndexByte(buf[bufpos:end], 0xFF)
			if ffpos == -1 {
				bufpos = end
				continue NEXTBLOCK
			}
			bufpos += uint32(ffpos)
			if bufpos == end-1 {
				// 2nd byte is in the next block.
				if _, err := reader.Seek(-1, io.SeekCurrent); err != nil {
					return nil, err
				}
				continue NEXTBLOCK
			}
			if buf[bufpos+1] == 0 {
				// Escaped 0xFF in data stream, delete
				// the 0.
				bufpos++
				copy(buf[bufpos:], buf[bufpos+1:end])
				skipped++
				end--
				continue NEXTINDEX
			}
			// Found a Marker.
			if _, err := reader.Seek(readpos+int64(bufpos+skipped), io.SeekStart); err != nil {
				return nil, err
			}
			return buf[:bufpos], nil
		}

	}
}

// WriteImageData writes a block of image data.
func WriteImageData(writer io.Writer, buf []byte) error {
	bufpos := 0
	tmp := []byte{0}
	for {
		ffpos := bytes.IndexByte(buf[bufpos:], 0xFF)
		if ffpos == -1 {
			_, err := writer.Write(buf[bufpos:])
			return err
		}
		if _, err := writer.Write(buf[bufpos : bufpos+ffpos+1]); err != nil {
			return err
		}
		// Escape a 0xFF byte by appending a 0 byte.
		if _, err := writer.Write(tmp); err != nil {
			return err
		}
		bufpos += ffpos + 1
	}
}

// Scanner represents a reader for JPEG markers and segments up to the
// SOS marker.
type Scanner struct {
	reader    io.ReadSeeker
	buf       []byte // buffer of size 2^16 - 3
	imageData bool   // true when expecting image data: after an SOS segment or RST marker.
}

// NewScanner creates a new Scanner and checks the JPEG header.
func NewScanner(reader io.ReadSeeker) (*Scanner, error) {
	scanner := new(Scanner)
	scanner.reader = reader
	scanner.buf = make([]byte, 2<<15-3)
	if err := ReadHeader(reader, scanner.buf); err != nil {
		return nil, err
	}
	return scanner, nil
}

// Scan reads the next JPEG data segment. Returns a zero Marker when
// image scan data is returned. Returns a nil slice if the marker as
// no segment data (RST0-7, EOI or TEM.)  The data buffer is only
// valid until Scan is called again.
func (scanner *Scanner) Scan() (Marker, []byte, error) {
	if scanner.imageData {
		var err error
		scanner.buf, err = ReadImageData(scanner.reader, scanner.buf)
		if err != nil {
			return 0, nil, err
		}
		if len(scanner.buf) == 0 {
			return 0, nil, errors.New("Expecting image data")
		}
		scanner.imageData = false
		return 0, scanner.buf, nil
	} else {
		marker, err := ReadMarker(scanner.reader, scanner.buf)
		if err != nil {
			return 0, nil, err
		}
		scanner.imageData = (marker == SOS || marker >= RST0 && marker <= RST0+7)
		if marker == EOI || marker == TEM || (marker >= RST0 && marker <= RST0+7) {
			return marker, nil, nil
		}
		segment, err := ReadData(scanner.reader, scanner.buf)
		return marker, segment, err
	}
}

// Dumper represents a writer for JPEG markers and segments.
type Dumper struct {
	writer io.Writer
}

// NewDumper creates a new Dumper and writes the JPEG header.
func NewDumper(writer io.Writer) (*Dumper, error) {
	dumper := new(Dumper)
	dumper.writer = writer
	if err := WriteMarker(writer, SOI); err != nil {
		return nil, err
	}
	return dumper, nil
}

// Dump writes a marker and its data segment.
func (dumper *Dumper) Dump(marker Marker, buf []byte) error {
	if marker == 0 {
		return WriteImageData(dumper.writer, buf)
	}
	if err := WriteMarker(dumper.writer, marker); err != nil {
		return err
	}
	if buf != nil {
		if err := WriteData(dumper.writer, buf); err != nil {
			return err
		}
	}
	return nil
}

// Segment represents a marker and its segment data.
type Segment struct {
	Marker Marker
	Data   []byte
}

// ReadSegments reads a JPEG stream up to and including the SOS marker and
// returns a slice with marker and segment data.
func ReadSegments(reader io.ReadSeeker) ([]Segment, error) {
	var segments = make([]Segment, 0, 20)
	scanner, err := NewScanner(reader)
	if err != nil {
		return segments, err
	}
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return segments, err
		}
		cpy := make([]byte, len(buf))
		copy(cpy, buf)
		segments = append(segments, Segment{marker, cpy})
		if marker == SOS {
			return segments, nil
		}
	}
}

// WriteSegments writes the given JPEG markers and segments to a stream.
func WriteSegments(writer io.WriteSeeker, segments []Segment) error {
	dumper, err := NewDumper(writer)
	if err != nil {
		return err
	}
	for i := range segments {
		if err := dumper.Dump(segments[i].Marker, segments[i].Data); err != nil {
			return err
		}
	}
	return nil
}

// Support for Multi-Picture Format (MPF), which specifies a way to
// store multiple images in a single JPEG file.

// MPFHeader is the text marker for a MPF segment, found in a JPEG
// APP2 segment.
var MPFHeader = []byte("MPF\000")

// MPFHeaderSize is the size of an MPF header.
const MPFHeaderSize = 4

// Tags in the MPFIndex IFD.
const (
	MPFVersion        = 0xB000
	MPFNumberOfImages = 0xB001
	MPFEntry          = 0xB002
	MPFImageUIDList   = 0xB003
	MPFTotalFrames    = 0xB004
)

// MPFIndexTagNames is a mapping from MPFIndex tags to strings.
var MPFIndexTagNames = map[tiff.Tag]string{
	MPFVersion:        "MPFVersion",
	MPFNumberOfImages: "MPFNumberOfImages",
	MPFEntry:          "MPFEntry",
	MPFImageUIDList:   "MPFImageUIDList",
	MPFTotalFrames:    "MPFTotalFrames",
}

// Tags in the MPFAttribute IFD.
const (
	// MPFVersion 0xB000 as above
	MPFIndividualImageNumber       = 0xB101
	MPFPanoramaScanningOrientation = 0xB201
	MPFPanoramaHorizontalOverlap   = 0xB202
	MPFPanoramaVerticalOverlap     = 0xB203
	MPFBaseViewpointNumber         = 0xB204
	MPFConvergenceAngle            = 0xB205
	MPFBaselineLength              = 0xB206
	MPFDivergenceAngle             = 0xB207
	MPFHorzontalAxisDistance       = 0xB208
	MPFVerticalAxisDistance        = 0xB209
	MPFCollimationAxisDistance     = 0xB20A
	MPFYawAngle                    = 0xB20B
	MPFPitchAngle                  = 0xB20C
	MPFRollAngle                   = 0xB20D
)

// MPFAttributeTagNames is a mapping from MPFAttribute tags to strings.
var MPFAttributeTagNames = map[tiff.Tag]string{
	MPFVersion:                     "MPFVersion",
	MPFIndividualImageNumber:       "MPFIndividualImageNumber",
	MPFPanoramaScanningOrientation: "MPFPanoramaScanningOrientation",
	MPFPanoramaHorizontalOverlap:   "MPFPanoramaHorizontalOverlap",
	MPFPanoramaVerticalOverlap:     "MPFPanoramaVerticalOverlap",
	MPFBaseViewpointNumber:         "MPFBaseViewpointNumber",
	MPFConvergenceAngle:            "MPFConvergenceAngle",
	MPFBaselineLength:              "MPFBaselineLength",
	MPFDivergenceAngle:             "MPFDivergenceAngle",
	MPFHorzontalAxisDistance:       "MPFHorzontalAxisDistance",
	MPFVerticalAxisDistance:        "MPFVerticalAxisDistance",
	MPFCollimationAxisDistance:     "MPFCollimationAxisDistance",
	MPFYawAngle:                    "MPFYawAngle",
	MPFPitchAngle:                  "MPFPitchAngle",
	MPFRollAngle:                   "MPFRollAngle",
}

// GetMPFHeader checks if a slice starts with an MPF header, as found
// in a JPEG APP2 segment.  Returns a flag and the position of the
// next byte.
func GetMPFHeader(buf []byte) (bool, uint32) {
	if uint32(len(buf)) >= MPFHeaderSize && bytes.Compare(buf[:MPFHeaderSize], MPFHeader) == 0 {
		return true, MPFHeaderSize
	} else {
		return false, 0
	}
}

// PutMPFHeader puts an MPF header at the start of a slice, returning
// the position of the next byte.
func PutMPFHeader(buf []byte) uint32 {
	copy(buf, MPFHeader)
	return MPFHeaderSize
}

// GetMPFTree reads a TIFF structure with MPF data. 'buf' must start
// with the first byte of the TIFF header. 'space' should be
// tiff.MPFIndexSpace for the first image in a file, and
// tiff.MPFAttributeSpace for subsequent images.
func GetMPFTree(buf []byte, space tiff.TagSpace) (*tiff.IFDNode, error) {
	valid, order, ifdpos := tiff.GetHeader(buf)
	if !valid {
		return nil, errors.New("GetMPFTree: Invalid Tiff header")
	}
	node, err := tiff.GetIFDTree(buf, order, ifdpos, space)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// MputMPFTree packs MPF data into a slice in TIFF format. The slice
// should start with the first byte following the MPF header. Returns
// the position following the last byte used.
func PutMPFTree(buf []byte, mpf *tiff.IFDNode) (uint32, error) {
	tiff.PutHeader(buf, mpf.Order, tiff.HeaderSize)
	return mpf.PutIFDTree(buf, tiff.HeaderSize)
}

// MakeMPFSegment serializes an MPF TIFF tree into a newly allocated
// slice, which can be used as an APP2 JPEG segment.
func MakeMPFSegment(tree *tiff.IFDNode) ([]byte, error) {
	size := MPFHeaderSize + tiff.HeaderSize + tree.TreeSize()
	buf := make([]byte, size)
	next := PutMPFHeader(buf)
	if _, err := PutMPFTree(buf[next:], tree); err != nil {
		return nil, err
	}
	return buf, nil
}

// MPFIndex holds the data from an MPF index segment about image
// locations in a file.
type MPFIndex struct {
	// MPF relative offset, from which MPF positions are measured:
	// the position after the MPF header in the file, which is 8
	// bytes from the start of the MPF APP2 block.
	Offset uint32
	// Offsets and lengths of images in the file. Offsets are
	// relative to the start of the file.
	ImageOffsets []uint32
	ImageLengths []uint32
}

// MPFIndexFromTIFF creates an MPFIndex struct from a TIFF node
// containing an MPF index and the MPF file offset.
func MPFIndexFromTIFF(node *tiff.IFDNode, offset uint32) (*MPFIndex, error) {
	var mpf MPFIndex
	mpf.Offset = offset
	order := node.Order
	count := uint32(0)
	var entryField tiff.Field
	for _, f := range node.Fields {
		switch f.Tag {
		case MPFNumberOfImages:
			count = f.Long(0, order)
		case MPFEntry:
			entryField = f
		}
	}
	if count == 0 {
		return nil, errors.New("MPF image count is 0")
	}
	if uint32(len(entryField.Data)) < 16*count {
		return nil, errors.New("MPF Entry doesn't have 16 bytes for each image")
	}
	offsets := make([]uint32, count)
	lengths := make([]uint32, count)
	for i := uint32(0); i < count; i++ {
		relOffset := entryField.Long(i*4+2, order)
		if relOffset != 0 {
			offsets[i] = relOffset + offset
			if offsets[i] < offset {
				return nil, errors.New("MPF offset overflow")
			}
		}
		if i == 0 {
			if offsets[i] != 0 {
				return nil, errors.New("First image should have an MPF offset of zero")
			}
		} else {
			if offsets[i] == 0 {
				return nil, errors.New("Only the first image should have an MPF offset of zero")
			}
		}
		lengths[i] = entryField.Long(i*4+1, order)
	}
	mpf.ImageOffsets = offsets
	mpf.ImageLengths = lengths
	return &mpf, nil
}

type MPFApply interface {
	MPFApply(reader io.ReadSeeker, index uint32, length uint32) error
}

// ImageIterate positions 'reader' at each image in turn, and calls
// the apply func with 'reader', the image index, and the image
// length.
func (mpf *MPFIndex) ImageIterate(reader io.ReadSeeker, apply MPFApply) error {
	for i := uint32(0); i < uint32(len(mpf.ImageOffsets)); i++ {
		if _, err := reader.Seek(int64(mpf.ImageOffsets[i]), io.SeekStart); err != nil {
			return err
		}
		if err := apply.MPFApply(reader, i, mpf.ImageLengths[i]); err != nil {
			return err
		}
	}
	return nil
}

// PutToTiff updates the file offsets and lengths in an MPF Tiff node
// with data from an MPFIndex structure.
func (mpf *MPFIndex) PutToTiff(node *tiff.IFDNode) {
	for _, f := range node.Fields {
		if f.Tag == MPFEntry {
			for i := 0; i < len(mpf.ImageOffsets); i++ {
				var offset uint32
				if mpf.ImageOffsets[i] > 0 {
					offset = mpf.ImageOffsets[i] - mpf.Offset
				}
				f.PutLong(offset, uint32(i*4+2), node.Order)
				f.PutLong(mpf.ImageLengths[i], uint32(i*4+1), node.Order)
			}
		}
	}
}

// MPFProcessor is an interface that provides a function for
// processing MPF APP2 blocks. It assumes that 'seg' is a slice
// containing a JPEG APP2 data segment, as returned by Scanner.Scan,
// and that 'reader' has just read that segment and is now positioned
// one byte past its end. 'writer' is either nil if not required, or
// an output stream to which we can write an APP2 marker and data
// segment. It returns a bool indicating whether an MPF block was
// processed, the APP2 data segment, possibly modified, and an error
// value.
type MPFProcessor interface {
	ProcessAPP2(writer io.WriteSeeker, reader io.ReadSeeker, seg []byte) (bool, []byte, error)
}

// MPFCheck conforms to the MPFProcessor interface. It checks for the
// presense of an MPF block without processing it in any way.
type MPFCheck struct {
}

func (MPFCheck) ProcessAPP2(_ io.WriteSeeker, _ io.ReadSeeker, seg []byte) (bool, []byte, error) {
	isMPF, _ := GetMPFHeader(seg)
	return isMPF, seg, nil
}

// MPFGetIndex conforms to the MPFProcessor interface. It can be
// applied to the first image in a file to read image positions from
// the MPF index.
type MPFGetIndex struct {
	Index *MPFIndex // MPF Index info.
}

func (mpfData *MPFGetIndex) ProcessAPP2(_ io.WriteSeeker, reader io.ReadSeeker, seg []byte) (bool, []byte, error) {
	isMPF, next := GetMPFHeader(seg)
	if isMPF {
		tree, err := GetMPFTree(seg[next:], tiff.MPFIndexSpace)
		if err != nil {
			return false, nil, err
		}
		// MPF offsets are relative to the byte following the
		// MPF header, which is 4 bytes past the start of seg.
		// The current position of the reader is one byte past
		// the data read into seg.
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, nil, err
		}
		offset := uint32(pos) - uint32(len(seg)-4)
		if mpfData.Index, err = MPFIndexFromTIFF(tree, offset); err != nil {
			return false, nil, err
		}
	}
	return isMPF, seg, nil
}

// MPFIndexRewriter conforms to the MPFProcessor interface. It can be
// applied to the first image in a file. It decodes the MPF index from
// TIFF format, and reencodes it to a new segment unchanged, recording
// the current write position. This allows the segment to be reencoded
// again later, possibly with different image positions, but still
// producing a segment of the same size.
type MPFIndexRewriter struct {
	Tree         *tiff.IFDNode // Unpacked MPF index TIFF tree.
	Index        *MPFIndex     // MPF Index info.
	APP2WritePos uint32        // Position of the MPF APP2 marker in the output stream.
}

func (mpfData *MPFIndexRewriter) ProcessAPP2(writer io.WriteSeeker, reader io.ReadSeeker, seg []byte) (bool, []byte, error) {
	isMPF, next := GetMPFHeader(seg)
	if isMPF {
		// copy the segment before decoding, since the tree
		// may contain pointers into it that will be
		// invalidated if the original slice is changed.
		saveseg := make([]byte, len(seg)-MPFHeaderSize)
		copy(saveseg, seg[next:])
		var err error
		mpfData.Tree, err = GetMPFTree(saveseg, tiff.MPFIndexSpace)
		if err != nil {
			return false, nil, err
		}
		mpfData.Tree.Fix()
		// MPF offsets are relative to the byte following the
		// MPF header, which is 4 bytes past the start of seg.
		// The current position of the reader is one byte past
		// the data read into seg.
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, nil, err
		}
		offset := uint32(pos) - uint32(len(seg)-4)
		if mpfData.Index, err = MPFIndexFromTIFF(mpfData.Tree, offset); err != nil {
			return false, nil, err
		}
		seg, err = MakeMPFSegment(mpfData.Tree)
		if err != nil {
			return false, nil, err
		}
		pos, err = writer.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, nil, err
		}
		mpfData.APP2WritePos = uint32(pos)
	}
	return isMPF, seg, nil
}

// SetMPFPositions modifies an MPF TIFF tree with new image offsets
// and sizes, given the offsets and the end of file position. It
// calculates the the lengths by assuming that the images are
// consecutive with no gaps.
func SetMPFPositions(mpfTree *tiff.IFDNode, mpfOffset uint32, offsets []uint32, end uint32) {
	count := len(offsets)
	lengths := make([]uint32, count)
	for i := 0; i < count-1; i++ {
		lengths[i] = offsets[i+1] - offsets[i]
	}
	lengths[count-1] = end - offsets[count-1]
	indexWrite := MPFIndex{mpfOffset, offsets, lengths}
	indexWrite.PutToTiff(mpfTree)
}

// RewriteMPF modifies an MPF TIFF tree with new image offsets and
// sizes, then overwrites the MPF data in the output stream at
// mpfWritePos. 'offsets' and 'end' as processed as per
// SetMPFPositions.
func RewriteMPF(writer io.WriteSeeker, mpfTree *tiff.IFDNode, mpfWritePos uint32, offsets []uint32, end uint32) error {
	SetMPFPositions(mpfTree, mpfWritePos+8, offsets, end)
	seg, err := MakeMPFSegment(mpfTree)
	if err != nil {
		return err
	}
	if _, err := writer.Seek(int64(mpfWritePos), io.SeekStart); err != nil {
		return err
	}
	if err := WriteMarker(writer, APP0+2); err != nil {
		return err
	}
	if err := WriteData(writer, seg); err != nil {
		return err
	}
	return nil
}




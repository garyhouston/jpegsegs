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
func WriteMarker(writer io.Writer, marker Marker, buf []byte) error {
	buf = buf[0:2]
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
func WriteData(writer io.Writer, buf []byte, lenbuf []byte) error {
	len := len(buf) + 2
	if len >= 2<<15 {
		return errors.New(fmt.Sprintf("writeData: data is too long (%d), max 2^16 - 3 (%d)", len-2, 2<<15-3))
	}
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
func WriteImageData(writer io.WriteSeeker, buf []byte) error {
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
	writer io.WriteSeeker
	buf    []byte // buffer of size 2
}

// NewDumper creates a new Dumper and writes the JPEG header.
func NewDumper(writer io.WriteSeeker) (*Dumper, error) {
	dumper := new(Dumper)
	dumper.writer = writer
	dumper.buf = make([]byte, 2)
	if err := WriteMarker(writer, SOI, dumper.buf); err != nil {
		return nil, err
	}
	return dumper, nil
}

// Dump writes a marker and its data segment.
func (dumper *Dumper) Dump(marker Marker, buf []byte) error {
	if marker == 0 {
		return WriteImageData(dumper.writer, buf)
	}
	if err := WriteMarker(dumper.writer, marker, dumper.buf); err != nil {
		return err
	}
	if buf != nil {
		if err := WriteData(dumper.writer, buf, dumper.buf); err != nil {
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

// MPF header, as found in a JPEG APP2 segment.
var mpfheader = []byte("MPF\000")

// MPFHeaderSize is the size of a MPF (Multi-Picture Format) header.
const MPFHeaderSize = 4

// GetMPFHeader checks if a slice starts with a Multi-Picture Format
// (MPF) header, as found in a JPEG APP2 segment.  Returns a flag and
// the position of the next byte.
func GetMPFHeader(buf []byte) (bool, uint32) {
	if uint32(len(buf)) >= MPFHeaderSize && bytes.Compare(buf[:MPFHeaderSize], mpfheader) == 0 {
		return true, MPFHeaderSize
	} else {
		return false, 0
	}
}

// PutMPFHeader puts a MPF header at the start of a slice, returning
// the position of the next byte.
func PutMPFHeader(buf []byte) uint32 {
	copy(buf, mpfheader)
	return MPFHeaderSize
}

// GetMPFTree reads a TIFF structure with MPF data. 'buf' must start
// with the first byte of the TIFF header.
func GetMPFTree(buf []byte) (*tiff.IFDNode, error) {
	valid, order, ifdpos := tiff.GetHeader(buf)
	if !valid {
		return nil, errors.New("GetMPFTree: Invalid Tiff header")
	}
	node, err := tiff.GetIFDTree(buf, order, ifdpos, tiff.MPFIndexSpace)
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

// MPFImageOffsets returns the file offset of each image referred to
// in an MPF index. Takes the unpacked MPF TIFF tree and the file
// offset of the MPF header.
func MPFImageOffsets(mpfTree *tiff.IFDNode, mpfOffset uint32) ([]uint32, error) {
	order := mpfTree.Order
	count := uint32(0)
	var entryField tiff.Field
	for _, f := range mpfTree.Fields {
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
	for i := uint32(0); i < count; i++ {
		relOffset := entryField.Long(i*4+2, order)
		if relOffset != 0 {
			offsets[i] = relOffset + mpfOffset
			if offsets[i] < mpfOffset {
				return nil, errors.New("MPF offset overflow")
			}
		}
	}
	return offsets, nil
}

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

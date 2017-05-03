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

// Size of a JPEG file header.
const HeaderSize = 2

// Indicate if buffer contains a JPEG header.
func IsJPEGHeader(buf []byte) bool {
	return buf[0] == 0xFF && buf[1] == SOI
}

// The JPEG header is a SOI marker. Filler bytes aren't allowed.
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

func WriteMarker(writer io.Writer, marker Marker, buf []byte) error {
	buf = buf[0:2]
	buf[0] = 0xFF
	buf[1] = byte(marker)
	_, err := writer.Write(buf)
	return err
}

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

// Read image scan data up to the next marker. 'buf' is either a
// buffer to read into, which will be reallocated if required, or nil
// to allocate a new buffer. Returns a buffer with the image data and
// the following marker.
func ReadImageData(reader io.ByteReader, buf []byte) ([]byte, Marker, error) {
	pos := 0
	ff := false
	if buf == nil {
		buf = make([]byte, 10000)
	} else {
		buf = buf[:cap(buf)]
	}
	for {
		var err error
		if buf[pos], err = reader.ReadByte(); err != nil {
			return buf[:pos], 0, err
		}
		if ff {
			if buf[pos] == 0 {
				// Escaped 0xFF in data stream, delete
				// the 0 by not incrementing pos.
				ff = false
				continue
			}
			// Marker
			return buf[:pos-1], Marker(buf[pos]), nil
		} else if buf[pos] == 0xFF {
			ff = true
		}
		pos++
		if cap(buf) < pos+1 {
			newbuf := make([]byte, 2*cap(buf))
			copy(newbuf, buf)
			buf = newbuf
		}
	}

}

// Write a block of image data.
func WriteImageData(writer io.ByteWriter, buf []byte) error {
	for pos := range buf {
		if err := writer.WriteByte(buf[pos]); err != nil {
			return err
		}
		if buf[pos] == 0xFF {
			if err := writer.WriteByte(0); err != nil {
				return err
			}
		}
		pos++
	}
	return nil
}

// Scanner represents a reader for JPEG markers and segments up to the
// SOS marker.
type Scanner struct {
	reader io.Reader
	buf    []byte // buffer of size 2^16 - 3
}

// NewScanner creates a new Scanner and checks the JPEG header.
func NewScanner(reader io.Reader) (*Scanner, error) {
	scanner := new(Scanner)
	scanner.reader = reader
	scanner.buf = make([]byte, 2<<15-3)
	if err := ReadHeader(reader, scanner.buf); err != nil {
		return nil, err
	}
	return scanner, nil
}

// Scan reads the next JPEG marker and its data segment. It doens't
// work past the SOS segment. The data buffer is only valid until Scan
// is called again.
func (scanner *Scanner) Scan() (Marker, []byte, error) {
	marker, err := ReadMarker(scanner.reader, scanner.buf)
	if err != nil {
		return 0, nil, err
	}
	segment, err := ReadData(scanner.reader, scanner.buf)
	return marker, segment, err

}

// Dumper represents a writer for JPEG markers and segments up to the SOS
// segment.
type Dumper struct {
	writer io.Writer
	buf    []byte // buffer of size 2
}

// NewDumper creates a new Dumper and writes the JPEG header.
func NewDumper(writer io.Writer) (*Dumper, error) {
	dumper := new(Dumper)
	dumper.writer = writer
	dumper.buf = make([]byte, 2)
	if err := WriteMarker(writer, SOI, dumper.buf); err != nil {
		return nil, err
	}
	return dumper, nil
}

// Dump writes a marker and its data segment from buf.
func (dumper *Dumper) Dump(marker Marker, buf []byte) error {
	if err := WriteMarker(dumper.writer, marker, dumper.buf); err != nil {
		return err
	}
	return WriteData(dumper.writer, buf, dumper.buf)
}

// Segment represents a marker and its segment data.
type Segment struct {
	Marker Marker
	Data   []byte
}

// ReadSegments reads a JPEG stream up to and including the SOS marker and
// returns a slice with marker and segment data.
func ReadSegments(reader io.Reader) ([]Segment, error) {
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
func WriteSegments(writer io.Writer, segments []Segment) error {
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

// Size of a MPF header.
const MPFHeaderSize = 4

// Check if a slice starts with a Multi-Picture Format (MPF) header,
// as found in a JPEG APP2 segment.  Returns a flag and the position
// of the next byte.
func GetMPFHeader(buf []byte) (bool, uint32) {
	if uint32(len(buf)) >= MPFHeaderSize && bytes.Compare(buf[:MPFHeaderSize], mpfheader) == 0 {
		return true, MPFHeaderSize
	} else {
		return false, 0
	}
}

// Put a MPF header, as for a JPEG APP2 segment, at the start of a slice,
// returning the position of the next byte.
func PutMPFHeader(buf []byte) uint32 {
	copy(buf, mpfheader)
	return MPFHeaderSize
}

// Read a TIFF structure with MPF data. 'buf' must start with the first byte
// of the TIFF header.
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

// Pack MPF data into a slice in TIFF format. The slice should start
// with the first byte following the MPF header. Returns the position
// following the last byte used.
func PutMPFTree(buf []byte, mpf *tiff.IFDNode) (uint32, error) {
	tiff.PutHeader(buf, mpf.Order, tiff.HeaderSize)
	return mpf.PutIFDTree(buf, tiff.HeaderSize)
}

// Tags in the MPFIndex IFD.
const (
	MPFVersion        = 0xB000
	MPFNumberOfImages = 0xB001
	MPFEntry          = 0xB002
	MPFImageUIDList   = 0xB003
	MPFTotalFrames    = 0xB004
)

// Mapping from MPFIndex tags to strings.
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

// Mapping from MPFAttribute tags to strings.
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

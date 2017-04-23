package jpegsegs

import (
	"errors"
	"fmt"
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
func readHeader(reader io.Reader, buf []byte) error {
	buf = buf[0:2]
	if _, err := io.ReadFull(reader, buf); err != nil {
		return err
	}
	if !IsJPEGHeader(buf) {
		return errors.New("SOI marker not found")
	}
	return nil
}

func readMarker(reader io.Reader, buf []byte) (Marker, error) {
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

func writeMarker(writer io.Writer, marker Marker, buf []byte) error {
	buf = buf[0:2]
	buf[0] = 0xFF
	buf[1] = byte(marker)
	_, err := writer.Write(buf)
	return err
}

func readData(reader io.Reader, buf []byte) ([]byte, error) {
	buf = buf[0:2]
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}
	length := int(buf[0])<<8 + int(buf[1]) - 2
	buf = buf[0:length]
	_, err := io.ReadFull(reader, buf)
	return buf, err
}

func writeData(writer io.Writer, buf []byte, lenbuf []byte) error {
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
func ReadImageData(reader io.Reader, buf []byte) ([]byte, Marker, error) {
	pos := 0
	ff := false
	if buf == nil {
		buf = make([]byte, 10000)
	} else {
		buf = buf[:cap(buf)]
	}
	for {
		if _, err := reader.Read(buf[pos : pos+1]); err != nil {
			return buf[:pos], 0, err
		}
		if ff {
			if buf[pos] == 0 {
				// 0xFF in data stream, delete the 0 by not
				// incrementing pos.
				ff = false
				continue
			}
			// Marker
			return buf[:pos-1], Marker(buf[pos]), nil
		}
		if buf[pos] == 0xFF {
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
	if err := readHeader(reader, scanner.buf); err != nil {
		return nil, err
	}
	return scanner, nil
}

// Scan reads the next JPEG marker and its data segment if it has one.
// All markers are expected to have data except for SOS, which indicates
// the start of scan data. Scan doesn't work past that point. The data
// buffer is only valid until Scan is called again.
func (scanner *Scanner) Scan() (Marker, []byte, error) {
	marker, err := readMarker(scanner.reader, scanner.buf)
	if err != nil {
		return 0, nil, err
	}
	if marker == SOS {
		return marker, nil, nil
	}
	segment, err := readData(scanner.reader, scanner.buf)
	return marker, segment, err

}

// Dumper represents a writer for JPEG markers and segments up to the SOS
// marker.
type Dumper struct {
	writer io.Writer
	buf    []byte // buffer of size 2
}

// NewDumper creates a new Dumper and writes the JPEG header.
func NewDumper(writer io.Writer) (*Dumper, error) {
	dumper := new(Dumper)
	dumper.writer = writer
	dumper.buf = make([]byte, 2)
	if err := writeMarker(writer, SOI, dumper.buf); err != nil {
		return nil, err
	}
	return dumper, nil
}

// Dump writes a marker and its data segment from buf. buf should be nil if
// it's the SOS marker (start of scan).
func (dumper *Dumper) Dump(marker Marker, buf []byte) error {
	if err := writeMarker(dumper.writer, marker, dumper.buf); err != nil {
		return err
	}
	if buf == nil {
		return nil
	}
	return writeData(dumper.writer, buf, dumper.buf)
}

// Copy reads all remaining data from a Scanner and lets the Dumper write it.
func (dumper *Dumper) Copy(scanner *Scanner) error {
	_, err := io.Copy(dumper.writer, scanner.reader)
	return err
}

// Segment represents a marker and its segment data.
type Segment struct {
	Marker Marker
	Data   []byte
}

// ReadAll reads a JPEG stream up to and including the SOS marker and
// returns a slice with marker and segment data. The SOS marker isn't
// included in the slice.
func ReadAll(reader io.Reader) (*Scanner, []Segment, error) {
	var segments = make([]Segment, 0, 20)
	scanner, err := NewScanner(reader)
	if err != nil {
		return nil, segments, err
	}
	for {
		marker, buf, err := scanner.Scan()
		if err != nil {
			return scanner, segments, err
		}
		if marker == SOS {
			return scanner, segments, nil
		}
		cpy := make([]byte, len(buf))
		copy(cpy, buf)
		segments = append(segments, Segment{marker, cpy})
	}
}

// WriteAll writes a JPEG stream up to and including the SOS marker, given
// a slice with marker and segment data. The SOS marker is written
// automatically, it should not be included in the slice.
func WriteAll(writer io.Writer, segments []Segment) (*Dumper, error) {
	dumper, err := NewDumper(writer)
	if err != nil {
		return nil, err
	}
	for i := range segments {
		if err := dumper.Dump(segments[i].Marker, segments[i].Data); err != nil {
			return dumper, err
		}
	}
	return dumper, dumper.Dump(SOS, nil)
}

# jpegsegs
jpegsegs is a Go library for reading and writing JPEG markers and segment data.

For documentation, see https://godoc.org/github.com/garyhouston/jpegsegs.

The segment data isn't decoded but could be further processed to extract
or modify metadata in formats such as Exif and XMP.

This library is still under construction and may change at any moment without backwards compatibility.

Example programs in the repository:

jpegsegsprint prints the markers and segment lengths in a JPEG file, including multiple images encoded with Multi-Picture Format (MPF) where present.

jpegsegscopy unpacks and repacks a JPEG file, making a copy that should be functionally identical, although not necessarily byte identical. It also supports MPF.

jpegsegsstrip makes a copy of a JPEG file with all COM, APP and JPG segments removed. Anything after the first EOI marker, including MPF additional images, is also removed.

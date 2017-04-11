/*
Package jpegsegs reads and writes JPEG markers and segment data. The
segment data isn't decoded but could be further processed to extract
or modify metadata in formats such as Exif and XMP.

Example programs are in the repository:

jpegsegsprint prints the markers and segment lengths for a JPEG file, up to th start of scan (SOS) marker.

jpegsegscopy makes an unmodified copy of a JPEG file.

jpegsegsstrip makes a copy of a JPEG file with all COM, APP and JPG segments removed.

*/
package jpegsegs

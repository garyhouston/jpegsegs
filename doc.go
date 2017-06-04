/*
Package jpegsegs reads and writes JPEG markers and segment data.  See the README in the repository for further notes.

The package contains functionality for basic JPEG files and for files that use Multi-Picture Format (MPF) to contain multiple images. Certain camera manufacturers use this format for steroscopic images or for "preview" images for display on a TV etc.

Reading and writing JPEG segments can be done by creating "scanners" and "dumpers", which wrap unbuffered seekable input and output streams. The example jpegsegsstrip program is a simple example: it doesn't need to decode information from MPF since it processes only the first image in a file and ignores additional images if present.

Processing files that use MPF is more complex. The MPF information is stored in APP2 segments in TIFF format; the MPF segment in the first file starts with index information. The index gives the offsets and lengths of the individual images. Reading the images can be done by unpacking the MPF index and seeking the input stream to each image in turn. This is demonstrated by the jpegsegsprint program.

Writing a multi-image file with MPF requires that the file positions of all images be encoded into the MPF index. The approach taken here is to initially write the index into the first image with nominal values, to reserve the appropriate amount of space in the APP2 segment. After all images have been written to the output, and the positions collected, the APP2 segment is then rewritten with the final positions. This is demonstrated by the jpegsegscopy program.
*/
package jpegsegs

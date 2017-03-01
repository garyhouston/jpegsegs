/*
Package jpegsegs reads and writes JPEG markers and segment data. The
segment data isn't decoded but could be further processed to extract
or modify metadata in formats such as Exif and XMP.

Example: Print the markers and segment lengths.

   package main

   import (
   	"bufio"
   	"fmt"
   	jseg "github.com/garyhouston/jpegsegs"
   	"os"
   )

   func main() {
   	if len(os.Args) != 2 {
   		fmt.Printf("Usage: %s file\n", os.Args[0])
   		return
   	}
   	in, err := os.Open(os.Args[1])
   	if err != nil {
   		panic(err)
   	}
   	reader := bufio.NewReader(in)
   	scanner, err := jseg.NewScanner(reader)
   	if err != nil {
   		panic(err)
   	}
   	for {
   		marker, buf, err := scanner.Scan()
   		if err != nil {
   			panic(err)
   		}
   		if marker == jseg.SOS {
   			break
   		}
   		fmt.Printf("%s, %d bytes\n", marker.Name(), len(buf))
   	}
   }

Example: Read the segments into memory and write back to another file.

   package main

   import (
   	"bufio"
   	"fmt"
   	jseg "github.com/garyhouston/jpegsegs"
   	"os"
   )

   func main() {
   	if len(os.Args) != 3 {
   		fmt.Printf("Usage: %s infile outfile\n", os.Args[0])
   		return
   	}
   	in, err := os.Open(os.Args[1])
   	if err != nil {
   		panic(err)
   	}
   	reader := bufio.NewReader(in)
   	scanner, segments, err := jseg.ReadAll(reader)
   	if err != nil {
   		panic(err)
   	}
   	out, err := os.Create(os.Args[2])
   	if err != nil {
   		panic(err)
   	}
   	writer := bufio.NewWriter(out)
   	dumper, err := jseg.WriteAll(writer, segments);
   	if err != nil {
   		panic(err)
   	}
	if err := dumper.Copy(scanner); err != nil {
		panic(err)
	}
   	if err := writer.Flush(); err != nil {
   		panic(err)
   	}
   }

Example: Strip COM, APP and JPG segments from a JPEG file.

   package main

   import (
   	"bufio"
   	"fmt"
   	jseg "github.com/garyhouston/jpegsegs"
   	"os"
   )

   func main() {
   	if len(os.Args) != 3 {
   		fmt.Printf("Usage: %s infile outfile\n", os.Args[0])
   		return
   	}
   	in, err := os.Open(os.Args[1])
   	if err != nil {
   		panic(err)
   	}
   	out, err := os.Create(os.Args[2])
   	if err != nil {
   		panic(err)
   	}
   	reader := bufio.NewReader(in)
   	writer := bufio.NewWriter(out)
   	scanner, err := jseg.NewScanner(reader)
   	if err != nil {
   		panic(err)
   	}
   	dumper, err := jseg.NewDumper(writer)
   	if err != nil {
   		panic(err)
   	}
   	for {
   		marker, buf, err := scanner.Scan()
   		if err != nil {
   			panic(err)
   		}
   		if marker == jseg.COM || marker >= jseg.APP0 && marker <= jseg.APP0+0xf || marker >= jseg.JPG0 && marker <= jseg.JPG0+0xD {
   			continue
   		}
   		if err := dumper.Dump(marker, buf); err != nil {
   			panic(err)
   		}
   		if marker == jseg.SOS {
   			break
   		}
   	}
   	if err := dumper.Copy(scanner); err != nil {
   		panic(err)
   	}
   	if err := writer.Flush(); err != nil {
   		panic(err)
   	}

   }

*/
package jpegsegs

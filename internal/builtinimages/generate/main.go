package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/alekzonder/tariboy/internal/builtinimages"
)

func main() {
	source := flag.String("source", "", "canonical basic image source directory")
	output := flag.String("output", "", "generated bundle output directory")
	version := flag.String("version", "", "owning tariboyd version")
	flag.Parse()
	if *source == "" || *output == "" || *version == "" {
		fmt.Fprintln(os.Stderr, "-source, -output, and -version are required")
		os.Exit(2)
	}
	if err := builtinimages.Generate(*source, *output, *version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// inspect is a small CLI for spot-checking the psx package's parsers
// against real WipEout 2097 asset files during development.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tridentsx/wipeout-go/internal/psx"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: inspect <file.TIM|file.CMP|file.PRM>")
		os.Exit(1)
	}
	path := os.Args[1]
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	switch strings.ToUpper(filepath.Ext(path)) {
	case ".TIM":
		inspectTIM(data)
	case ".CMP":
		inspectCMP(data)
	case ".PRM":
		inspectPRM(data)
	default:
		fmt.Fprintln(os.Stderr, "unknown extension:", filepath.Ext(path))
		os.Exit(1)
	}
}

func inspectTIM(data []byte) {
	img, err := psx.DecodeTIM(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	nonZeroAlpha := 0
	for i := 3; i < len(img.Pixels); i += 4 {
		if img.Pixels[i] != 0 {
			nonZeroAlpha++
		}
	}
	fmt.Printf("TIM: %dx%d, %d opaque px of %d total\n", img.Width, img.Height, nonZeroAlpha, img.Width*img.Height)
}

func inspectCMP(data []byte) {
	files, err := psx.DecodeCMP(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("CMP: %d member file(s)\n", len(files))
	for i, f := range files {
		fmt.Printf("  [%d] %d bytes\n", i, len(f))
		if img, err := psx.DecodeTIM(f); err == nil {
			fmt.Printf("      -> decodes as TIM: %dx%d\n", img.Width, img.Height)
		}
	}
}

func inspectPRM(data []byte) {
	objects, err := psx.DecodePRM(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("PRM: %d object(s)\n", len(objects))
	for i, obj := range objects {
		fmt.Printf("  [%d] name=%q vertices=%d polygons=%d origin=%+v position=%+v\n",
			i, obj.Header.Name, len(obj.Vertices), len(obj.Polygons), obj.Header.Origin, obj.Header.Position)
		typeCounts := map[uint16]int{}
		for _, p := range obj.Polygons {
			typeCounts[p.Type]++
		}
		fmt.Printf("      polygon types: %v\n", typeCounts)
	}
}

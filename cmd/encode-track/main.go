// Command encode-track packages a WipEout 2097 track into a self-describing
// "trackpack" — sky + surrounding scenery + the driving surface with its tiles
// and gameplay triggers — as modern, upres-ready assets. See
// docs/track-format.md for the format.
//
//	go run ./cmd/encode-track --all              # every track -> <out>/<TRACK>.trackpack
//	go run ./cmd/encode-track TRACK01            # a single track
//
// The driving surface stays logical (faces keep their tile index + flags), so
// no gameplay trigger is baked away; scenery/sky are baked glTF meshes. All
// geometry is emitted in glTF space (x,-y,-z, Y-up) so the layers align.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/glb"
	"github.com/tridentsx/wipeout-go/internal/model"
	"github.com/tridentsx/wipeout-go/internal/psx"
	"github.com/tridentsx/wipeout-go/internal/trackpack"
)

func main() {
	disc := flag.String("disc", "assets/WIPEOUT2", "extracted WipEout 2097 disc root")
	out := flag.String("out", "export-tracks", "output directory for .trackpack packs")
	all := flag.Bool("all", false, "encode every track under -disc")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage:\n  %s --all [-disc DIR] [-out DIR]\n  %s [-disc DIR] [-out DIR] TRACKNN\n\n"+
				"Packages a whole track (sky + scenery + driving surface with tiles/triggers)\n"+
				"into <out>/<TRACK>.trackpack. See docs/track-format.md.\n\n",
			filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	loader := assets.Loader{Root: *disc}
	if *all {
		names, err := trackNames(*disc)
		if err != nil {
			fatal(err)
		}
		failures := 0
		for _, name := range names {
			if err := encodeTrack(loader, name, *out); err != nil {
				fmt.Fprintln(os.Stderr, "FAILED:", name, err)
				failures++
			}
		}
		fmt.Fprintf(os.Stderr, "encoded %d track(s) to %s\n", len(names)-failures, *out)
		if failures > 0 {
			os.Exit(1)
		}
		return
	}

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := encodeTrack(loader, flag.Arg(0), *out); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", filepath.Join(*out, flag.Arg(0)+".trackpack"))
}

// trackNames finds track directories (those with a TRACK.TRV).
func trackNames(disc string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(disc, "*", "TRACK.TRV"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, m := range matches {
		names = append(names, filepath.Base(filepath.Dir(m)))
	}
	sort.Strings(names)
	return names, nil
}

func encodeTrack(loader assets.Loader, name, outRoot string) error {
	surface, err := loader.LoadTrackSurface(name)
	if err != nil {
		return err
	}

	packDir := filepath.Join(outRoot, name+".trackpack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return err
	}

	p := trackpack.Pack{FormatVersion: 1, Name: name, Axes: "gltf"}
	p.Surface = buildSurface(surface)

	// Surface tiles: one PNG per logical tile index (upres-swappable).
	tileDir := filepath.Join(packDir, "surface", "tiles")
	if err := os.MkdirAll(tileDir, 0o755); err != nil {
		return err
	}
	for i, tile := range surface.Tiles {
		file := filepath.Join("surface", "tiles", fmt.Sprintf("%03d.png", i))
		if err := writePNG(filepath.Join(packDir, file), tile); err != nil {
			return err
		}
		p.Textures.Surface = append(p.Textures.Surface, trackpack.SurfaceTexture{
			Tile: i, File: filepath.ToSlash(file), Width: tile.Width, Height: tile.Height,
		})
	}

	// Scenery + sky: baked glTF meshes (embedded textures).
	if ref, texRef, err := encodeLayer(loader, name, "SCENE.PRM", packDir, "scenery.glb"); err != nil {
		return err
	} else if ref != nil {
		p.Layers.Scenery, p.Textures.Scenery = ref, texRef
	}
	if ref, texRef, err := encodeLayer(loader, name, "SKY.PRM", packDir, "sky.glb"); err != nil {
		return err
	} else if ref != nil {
		p.Layers.Sky, p.Textures.Sky = ref, texRef
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(packDir, "track.json"), data, 0o644)
}

// encodeLayer bakes a PRM layer (scenery or sky) to a .glb inside the pack.
func encodeLayer(loader assets.Loader, name, prm, packDir, outName string) (*trackpack.LayerRef, *trackpack.EmbeddedTexture, error) {
	m, err := loader.LoadModel(name, prm)
	if err != nil {
		// A track may lack a layer; treat as optional.
		return nil, nil, nil
	}
	for _, w := range m.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	mesh := model.FromPRM(m.Objects, model.PageSizes(m.Pages))
	doc, err := glb.BuildDocument(name+"_"+m.Name, mesh, m.Pages, glb.DefaultOptions())
	if err != nil {
		return nil, nil, err
	}
	if err := glb.Save(filepath.Join(packDir, outName), doc); err != nil {
		return nil, nil, err
	}
	source := prm[:len(prm)-len(filepath.Ext(prm))] + ".CMP"
	return &trackpack.LayerRef{File: outName}, &trackpack.EmbeddedTexture{Source: source, EmbeddedIn: outName}, nil
}

func writePNG(path string, img *psx.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	nrgba := &image.NRGBA{Pix: img.Pixels, Stride: img.Width * 4, Rect: image.Rect(0, 0, img.Width, img.Height)}
	return png.Encode(f, nrgba)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "encode-track:", err)
	os.Exit(1)
}

// Command export-models exports WipEout 2097 .PRM models to binary glTF (.glb)
// at highest fidelity: geometry, face/vertex colors, embedded textures (from
// each model's paired .CMP), and sprites.
//
//	go run ./cmd/export-models --all                 # every runtime PRM
//	go run ./cmd/export-models COMMON/VECTO.PRM       # a single model
//	go run ./cmd/export-models --split COMMON/JUNE.PRM // one .glb per object
//
// It reads only the extracted disc tree and writes .glb files; it uses no SDL.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tridentsx/wipeout-go/internal/assets"
	"github.com/tridentsx/wipeout-go/internal/glb"
	"github.com/tridentsx/wipeout-go/internal/model"
	"github.com/tridentsx/wipeout-go/internal/psx"
)

// editorPRMs are the three development/interchange-layout PRMs the retail
// parser deliberately does not decode; --all skips them.
var editorPRMs = map[string]bool{
	"COMMON/SKY.PRM":    true,
	"COMMON/TRACK.PRM":  true,
	"TRACK08/TRAK2.PRM": true,
}

// layoutMode selects how a multi-object PRM is written.
type layoutMode int

const (
	// layoutAuto writes one file per object only when the objects form a
	// stacked "collection" (mesh.IsCollection); coherent scenes stay merged.
	layoutAuto layoutMode = iota
	// layoutSplit forces one file per object for every multi-object PRM.
	layoutSplit
	// layoutMerge forces a single file per PRM (objects placed by Position).
	layoutMerge
)

func main() {
	disc := flag.String("disc", "assets/WIPEOUT2", "extracted WipEout 2097 disc root")
	out := flag.String("out", "export", "output directory for .glb files")
	all := flag.Bool("all", false, "export every runtime PRM found under -disc")
	split := flag.Bool("split", false, "force one .glb per object for every multi-object PRM")
	merge := flag.Bool("merge", false, "force a single .glb per PRM (disable auto collection splitting)")
	singleSided := flag.Bool("single-sided", false, "cull back faces (default: double-sided)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage:\n  %s --all [-disc DIR] [-out DIR]\n  %s [-out DIR] PRM\n\n"+
				"Exports .PRM models to highest-fidelity .glb (geometry, colors, textures, sprites).\n"+
				"PRM may be a filesystem path or a path relative to -disc (e.g. COMMON/VECTO.PRM).\n\n"+
				"By default, PRMs whose objects are stacked previews (e.g. COMMON/JUNE, HARRY, TERRY)\n"+
				"are auto-split into one file per object under <out>/<NAME>/; coherent scenes\n"+
				"(TRACK*/SCENE.PRM, COMMON/TRAIN) stay a single merged file. Use -split or -merge to override.\n\n",
			filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if *split && *merge {
		fatal(fmt.Errorf("-split and -merge are mutually exclusive"))
	}
	mode := layoutAuto
	switch {
	case *split:
		mode = layoutSplit
	case *merge:
		mode = layoutMerge
	}

	opts := glb.DefaultOptions()
	if *singleSided {
		opts.DoubleSided = false
	}

	if *all {
		if flag.NArg() != 0 {
			flag.Usage()
			os.Exit(2)
		}
		if err := exportAll(*disc, *out, opts, mode); err != nil {
			fatal(err)
		}
		return
	}

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	prm := flag.Arg(0)
	if _, err := os.Stat(prm); err != nil {
		prm = filepath.Join(*disc, prm)
	}
	name, files, err := exportOne(prm, *out, opts, mode)
	if err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %d file(s) for %s under %s\n", files, name, *out)
}

// exportOne loads a single PRM (by filesystem path) and writes it under outDir.
// It returns the model name and the number of .glb files written.
func exportOne(prmPath, outDir string, opts glb.Options, mode layoutMode) (string, int, error) {
	loader := assets.Loader{Root: filepath.Dir(prmPath)}
	m, err := loader.LoadModel(filepath.Base(prmPath))
	if err != nil {
		return "", 0, err
	}
	for _, w := range m.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	n, err := writeModel(m, outDir, opts, mode)
	return m.Name, n, err
}

// exportAll walks the disc root and exports every runtime PRM, preserving the
// per-directory layout under outDir so same-named files (SCENE/SKY per track)
// do not collide.
func exportAll(disc, outDir string, opts glb.Options, mode layoutMode) error {
	models, files, skipped := 0, 0, 0
	var failures []string
	err := filepath.WalkDir(disc, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".PRM") {
			return nil
		}
		rel, relErr := filepath.Rel(disc, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if editorPRMs[rel] {
			skipped++
			return nil
		}
		loader := assets.Loader{Root: filepath.Dir(path)}
		m, loadErr := loader.LoadModel(filepath.Base(path))
		if loadErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", rel, loadErr))
			return nil
		}
		for _, w := range m.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		destDir := filepath.Join(outDir, filepath.FromSlash(filepath.Dir(rel)))
		n, writeErr := writeModel(m, destDir, opts, mode)
		if writeErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", rel, writeErr))
			return nil
		}
		models++
		files += n
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "exported %d model(s) -> %d file(s) in %s (skipped %d editor PRM(s))\n", models, files, outDir, skipped)
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, "FAILED:", f)
		}
		return fmt.Errorf("%d model(s) failed to export", len(failures))
	}
	return nil
}

// writeModel writes m into destDir. It writes one <destDir>/<name>.glb with
// every object placed at its world Position, unless the objects are split into
// one file per object under <destDir>/<name>/NN_<object>.glb. Splitting happens
// when mode is layoutSplit, or (layoutAuto) when the objects form a stacked
// collection -- collection PRMs (ship/menu/track previews) author each object
// at the origin, so a single combined file overlaps them. It returns the number
// of .glb files written.
func writeModel(m *assets.Model, destDir string, opts glb.Options, mode layoutMode) (int, error) {
	mesh := model.FromPRM(m.Objects, model.PageSizes(m.Pages))

	split := false
	switch mode {
	case layoutSplit:
		split = true
	case layoutAuto:
		split = mesh.IsCollection()
	}

	if split && len(mesh.Objects) > 1 {
		base := filepath.Join(destDir, m.Name)
		if err := os.MkdirAll(base, 0o755); err != nil {
			return 0, err
		}
		for i := range mesh.Objects {
			sub, used := model.ObjectMesh(mesh.Objects[i])
			doc, err := glb.BuildDocument(fmt.Sprintf("%s_%02d", m.Name, i), sub, selectPages(m.Pages, used), opts)
			if err != nil {
				return i, err
			}
			out := filepath.Join(base, fmt.Sprintf("%02d_%s.glb", i, sanitizeName(mesh.Objects[i].Name)))
			if err := glb.Save(out, doc); err != nil {
				return i, err
			}
		}
		return len(mesh.Objects), nil
	}

	doc, err := glb.BuildDocument(m.Name, mesh, m.Pages, opts)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, err
	}
	return 1, glb.Save(filepath.Join(destDir, m.Name+".glb"), doc)
}

// selectPages picks the texture images at the given original indices, matching
// model.ObjectMesh's compacted page order.
func selectPages(pages []*psx.Image, used []int) []*psx.Image {
	out := make([]*psx.Image, len(used))
	for i, idx := range used {
		if idx >= 0 && idx < len(pages) {
			out[i] = pages[idx]
		}
	}
	return out
}

// sanitizeName makes an object name safe for a filename.
func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "object"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "export-models:", err)
	os.Exit(1)
}

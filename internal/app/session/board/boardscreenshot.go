// A picture of a board's display, written to disk.
//
// board.screen answers "did anything change" as a number, which is enough for
// a script driving a menu. This is the other half: the frame as a PNG, for the
// times the question is "show me" - a proof a firmware navigates, a bug report,
// a screenshot in a doc. It reads the same framebuffer the panel model holds
// and encodes it here rather than anywhere near a desktop, because the honest
// picture is the one the firmware drew, not the one a window manager composited.
package board

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"
)

func registerBoardScreenshot(st *state.Store, s *session.Sim) {
	// board.screenshot: the board's display as a PNG under the node's work
	// directory. The path is returned so a caller can open it; the frame is
	// exactly what the controller holds, at the size it holds it.
	st.HandleSpec("board.screenshot", state.Spec{
		What: "write the board's display to a PNG and return its path",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node whose screen to capture"},
		},
		Returns: []string{"node", "path", "width", "height", "bpp", "on"},
	}, func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "node")
		if name == "" {
			return nil, fmt.Errorf("board.screenshot needs a node")
		}
		n, found := s.LiveEngine().NodeByName(name)
		if !found || n.Firmware == nil {
			return nil, fmt.Errorf("%s is not running", name)
		}
		sc, ok := n.Firmware.Backend.(interface {
			Screen() (int, int, int, bool, []byte, bool)
		})
		if !ok {
			return nil, fmt.Errorf("%s is not a board with a display", name)
		}
		width, height, bpp, on, bits, have := sc.Screen()
		if !have {
			return nil, fmt.Errorf("%s has drawn nothing yet", name)
		}
		img, err := frameToImage(width, height, bpp, bits)
		if err != nil {
			return nil, err
		}
		// NodeWorkDir maps the name to [a-zA-Z0-9_-] and the filename is a
		// constant, so this path is the node's own directory and nowhere else.
		path := filepath.Join(firmware.NodeWorkDir(name), "screen.png")
		f, err := os.Create(path) //nolint:gosec // a sanitised path this package composed
		if err != nil {
			return nil, fmt.Errorf("board.screenshot: %w", err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("board.screenshot: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("board.screenshot: %w", err)
		}
		return map[string]any{"node": name, "path": path,
			"width": width, "height": height, "bpp": bpp, "on": on}, nil
	})
}

// frameToImage turns a controller's framebuffer into an RGBA picture. A colour
// panel is RGB565 two bytes a pixel; a monochrome one is page-ordered bits, lit
// pixels drawn white on black so the picture reads the way the glass does.
func frameToImage(width, height, bpp int, bits []byte) (image.Image, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("board.screenshot: a %dx%d frame has no picture", width, height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	switch bpp {
	case 16:
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, g, b, ok := peripheral.RGB565At(bits, width, x, y)
				if !ok {
					r, g, b = 0, 0, 0
				}
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 0xFF})
			}
		}
	case 1:
		f := &peripheral.PanelFrame{Width: width, Height: height, BPP: 1, Bits: bits}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				v := uint8(0)
				if f.Lit(x, y) {
					v = 0xFF
				}
				img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 0xFF})
			}
		}
	default:
		return nil, fmt.Errorf("board.screenshot: a %d-bit panel is not one this draws", bpp)
	}
	return img, nil
}

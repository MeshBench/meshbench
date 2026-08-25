package session

import (
	"image/color"
	"testing"
)

// A 2x1 RGB565 frame of pure red then pure blue, widened to 8-bit channels,
// is the picture the encoder must produce - the byte order low-first is the
// thing most easily got wrong, so the test pins it.
func TestFrameToImageRGB565(t *testing.T) {
	// RGB565: red = 0xF800, blue = 0x001F, low byte first.
	bits := []byte{0x00, 0xF8, 0x1F, 0x00}
	img, err := frameToImage(2, 1, 16, bits)
	if err != nil {
		t.Fatalf("frameToImage: %v", err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 0xFF || g != 0 || b != 0 {
		t.Errorf("pixel 0 not red: got %02x%02x%02x", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(1, 0).RGBA()
	if r != 0 || g != 0 || b>>8 != 0xFF {
		t.Errorf("pixel 1 not blue: got %02x%02x%02x", r>>8, g>>8, b>>8)
	}
}

// A monochrome frame is page-ordered: byte n holds eight vertical pixels of
// column n. Bit 0 set means the top pixel is lit; it must come back white.
func TestFrameToImageMono(t *testing.T) {
	// 1 wide, 8 tall, one byte: only the top pixel lit.
	img, err := frameToImage(1, 8, 1, []byte{0x01})
	if err != nil {
		t.Fatalf("frameToImage: %v", err)
	}
	if got := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA); got.R != 0xFF {
		t.Errorf("top pixel not lit: %+v", got)
	}
	if got := color.RGBAModel.Convert(img.At(0, 1)).(color.RGBA); got.R != 0 {
		t.Errorf("second pixel should be dark: %+v", got)
	}
}

func TestFrameToImageRejectsBadBPP(t *testing.T) {
	if _, err := frameToImage(2, 2, 7, make([]byte, 8)); err == nil {
		t.Error("a 7-bit panel should be refused")
	}
	if _, err := frameToImage(0, 0, 16, nil); err == nil {
		t.Error("a zero-size frame should be refused")
	}
}

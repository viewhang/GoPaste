package native

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// TestThumbnail 验证降采样后的尺寸、方向与半透明像素
func TestThumbnail(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := range 200 {
		for x := range 400 {
			col := color.RGBA{R: 255, A: 255}
			if y >= 100 {
				col = color.RGBA{B: 128, A: 128}
			}
			src.SetRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	img, err := Thumbnail(buf.Bytes(), 40)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 40, 20) {
		t.Fatalf("缩略图尺寸错误：%v", img.Bounds())
	}
	if top, bottom := img.RGBAAt(20, 2), img.RGBAAt(20, 17); top.R < 250 || top.A != 255 || bottom.B < 125 || bottom.A < 125 || bottom.A > 130 {
		t.Fatalf("缩略图方向或透明度错误：上方 %v，下方 %v", top, bottom)
	}
	if _, err := Thumbnail(nil, 40); err == nil {
		t.Fatal("缺失图片未返回错误")
	}
}

// TestThumbnailEXIF 验证 JPEG 降采样保留相机记录的旋转方向
func TestThumbnailEXIF(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := range 200 {
		for x := range 400 {
			col := color.RGBA{R: 255, A: 255}
			if y >= 100 {
				col = color.RGBA{B: 255, A: 255}
			}
			src.SetRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	exif := []byte{0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	data := append([]byte{0xff, 0xd8}, exif...)
	data = append(data, buf.Bytes()[2:]...)
	img, err := Thumbnail(data, 40)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 20, 40) || img.RGBAAt(2, 20).B < 240 || img.RGBAAt(17, 20).R < 240 {
		t.Fatal("JPEG 的 EXIF 旋转方向丢失")
	}
}

package designshare

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func box(kind string, parts ...[]byte) []byte {
	size := 8
	for _, p := range parts {
		size += len(p)
	}
	out := binary.BigEndian.AppendUint32(nil, uint32(size))
	out = append(out, kind...)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func mp4Of(seconds uint32, handlers ...string) []byte {
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:], 600)
	binary.BigEndian.PutUint32(mvhd[16:], seconds*600)
	moov := [][]byte{box("mvhd", mvhd)}
	for _, h := range handlers {
		hdlr := append(make([]byte, 8), h...)
		hdlr = append(hdlr, make([]byte, 12)...)
		moov = append(moov, box("trak", box("mdia", box("hdlr", hdlr))))
	}
	return append(box("ftyp", []byte("isom")), box("moov", moov...)...)
}

// What a video may be, whatever a host claims: no sound, a length, an MP4 at all.
func TestCheckVideo(t *testing.T) {
	if err := CheckVideo(mp4Of(5, "vide"), 15); err != nil {
		t.Fatalf("a video: %v", err)
	}
	long := mp4Of(5, "vide")
	for name, data := range map[string][]byte{
		"empty":         nil,
		"not one":       []byte("<script>alert(1)</script>"),
		"html in a box": box("ftyp", []byte("<html>")),
		"sound":         mp4Of(5, "vide", "soun"),
		"too long":      mp4Of(16, "vide"),
		"no length":     mp4Of(0, "vide"),
		"no tracks":     mp4Of(5),
		"cut short":     long[:len(long)-3],
		"lying size":    append(binary.BigEndian.AppendUint32(nil, 0xffffffff), "ftypisom"...),
		"moov first":    box("moov", box("trak", box("mdia", box("hdlr", append(make([]byte, 8), "vide"...))))),
	} {
		if err := CheckVideo(data, 15); err == nil {
			t.Fatalf("%s passed", name)
		}
	}
}

// The real thing, as the app's encoder makes it, with and without a sound track.
func TestCheckVideoOfFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg here")
	}
	dir := t.TempDir()
	make := func(name string, args ...string) []byte {
		t.Helper()
		out := filepath.Join(dir, name)
		cmd := exec.Command(ff, append(append([]string{"-y", "-hide_banner", "-loglevel", "error"}, args...), "-movflags", "+faststart", "-f", "mp4", out)...)
		if msg, err := cmd.CombinedOutput(); err != nil {
			if strings.Contains(string(msg), "Unknown encoder") || strings.Contains(string(msg), "not found") {
				t.Skipf("this ffmpeg can't: %s", msg)
			}
			t.Fatalf("ffmpeg: %v: %s", err, msg)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	silent := make("silent.mp4", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=10:d=2", "-an", "-pix_fmt", "yuv420p")
	if err := CheckVideo(silent, 15); err != nil {
		t.Fatalf("what the encoder makes: %v", err)
	}
	if err := CheckVideo(silent, 1); err == nil {
		t.Fatal("over its length, passed")
	}
	loud := make("loud.mp4", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=10:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-shortest", "-pix_fmt", "yuv420p")
	if err := CheckVideo(loud, 15); err == nil {
		t.Fatal("a video with sound passed")
	}
}

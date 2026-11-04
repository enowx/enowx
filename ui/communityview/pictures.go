package communityview

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strconv"

	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/webp" // what the library's pictures are

	"github.com/enowx/enowx/proto"
)

// picturesAtOnce is how many pictures are on their way at once: the cloud limits
// what it reads at a time, and a grid shouldn't take it all.
const picturesAtOnce = 3

// picturesKept is how many stay decoded; past it the oldest go, and come again
// if they show again.
const picturesKept = 120

// maxPictureSide is the widest or tallest picture decoded: the cloud's limit is
// 1200 wide, and a picture is someone else's file, so its header is looked at
// before memory is given to it.
const maxPictureSide = 4096

// pictures holds what the grid shows, on the UI's goroutine.
type pictures struct {
	got     map[string]*picture
	order   []string
	loading int
}

type picture struct {
	bmp   *ui.Bitmap
	asked bool
}

// of is it's picture if it's here; it's asked for when there's a place.
func (p *pictures) of(o Options, it proto.LibraryListing) *ui.Bitmap {
	if p.got == nil {
		p.got = map[string]*picture{}
	}
	key := it.ItemID + "/" + strconv.Itoa(it.Version)
	pic := p.got[key]
	if pic == nil {
		pic = &picture{}
		p.got[key] = pic
		p.order = append(p.order, key)
		if len(p.order) > picturesKept {
			delete(p.got, p.order[0])
			p.order = p.order[1:]
		}
	}
	if !pic.asked && o.Picture != nil && p.loading < picturesAtOnce {
		pic.asked = true
		p.loading++
		o.Picture(it, func(bmp *ui.Bitmap, err error) {
			p.loading--
			if err == nil {
				pic.bmp = bmp
			}
		})
	}
	return pic.bmp
}

// DecodePicture is a picture from the library as the view shows it. Call it off
// the UI's goroutine.
func DecodePicture(data []byte) (*ui.Bitmap, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("community: a picture: %w", err)
	}
	if cfg.Width > maxPictureSide || cfg.Height > maxPictureSide {
		return nil, errors.New("community: a picture is too large")
	}
	return ui.DecodeBitmap(data)
}

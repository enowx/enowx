package designshare

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// A design's short video is an MP4 with one or more video tracks and nothing else:
// the host and the cloud both read its boxes before it goes on, so that a preview
// can't carry sound or be longer than it should, whatever the player does.

// mp4Box calls fn with each box of data, its type and its contents.
func mp4Box(data []byte, fn func(kind string, body []byte) error) error {
	for len(data) > 0 {
		if len(data) < 8 {
			return errors.New("designshare: a video's box is cut short")
		}
		size := uint64(binary.BigEndian.Uint32(data))
		kind, head := string(data[4:8]), uint64(8)
		switch size {
		case 0:
			size = uint64(len(data))
		case 1:
			if len(data) < 16 {
				return errors.New("designshare: a video's box is cut short")
			}
			size, head = binary.BigEndian.Uint64(data[8:]), 16
		}
		if size < head || size > uint64(len(data)) {
			return errors.New("designshare: a video's box is the wrong size")
		}
		if err := fn(kind, data[head:size]); err != nil {
			return err
		}
		data = data[size:]
	}
	return nil
}

// mp4Handler is what a trak box holds: "vide" for video, "soun" for sound.
func mp4Handler(trak []byte) (string, error) {
	handler := ""
	err := mp4Box(trak, func(kind string, body []byte) error {
		if kind != "mdia" {
			return nil
		}
		return mp4Box(body, func(kind string, body []byte) error {
			if kind == "hdlr" && len(body) >= 12 {
				handler = string(body[8:12])
			}
			return nil
		})
	})
	return handler, err
}

// mp4Seconds reads an mvhd box's length.
func mp4Seconds(mvhd []byte) (float64, error) {
	if len(mvhd) < 1 {
		return 0, errors.New("designshare: a video's header is cut short")
	}
	var scale, length uint64
	if mvhd[0] == 1 {
		if len(mvhd) < 32 {
			return 0, errors.New("designshare: a video's header is cut short")
		}
		scale, length = uint64(binary.BigEndian.Uint32(mvhd[20:])), binary.BigEndian.Uint64(mvhd[24:])
	} else {
		if len(mvhd) < 20 {
			return 0, errors.New("designshare: a video's header is cut short")
		}
		scale, length = uint64(binary.BigEndian.Uint32(mvhd[12:])), uint64(binary.BigEndian.Uint32(mvhd[16:]))
	}
	if scale == 0 {
		return 0, errors.New("designshare: a video's header has no time scale")
	}
	return float64(length) / float64(scale), nil
}

// CheckVideo says data is an MP4 of video alone, with a length, at most maxSeconds.
func CheckVideo(data []byte, maxSeconds float64) error {
	first, found, videos := "", false, 0
	seconds := 0.0
	err := mp4Box(data, func(kind string, body []byte) error {
		if first == "" {
			first = kind
		}
		if kind != "moov" {
			return nil
		}
		found = true
		return mp4Box(body, func(kind string, body []byte) error {
			switch kind {
			case "mvhd":
				var err error
				seconds, err = mp4Seconds(body)
				return err
			case "trak":
				handler, err := mp4Handler(body)
				if err != nil {
					return err
				}
				if handler != "vide" {
					return errors.New("designshare: a video has a track that isn't video: no sound")
				}
				videos++
			}
			return nil
		})
	})
	switch {
	case err != nil:
		return err
	case first != "ftyp" || !found:
		return errors.New("designshare: that isn't an MP4")
	case videos == 0:
		return errors.New("designshare: this MP4 has no video")
	case seconds <= 0:
		return errors.New("designshare: this MP4 has no length")
	case seconds > maxSeconds:
		return fmt.Errorf("designshare: a video is %.0f seconds at most", maxSeconds)
	}
	return nil
}

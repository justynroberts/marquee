package camera

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework Foundation -framework Vision
#include "camera_darwin.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

const maxW, maxH = C.MQ_CAM_W, C.MQ_CAM_MAXH

func start() error {
	var buf [256]C.char
	switch C.mq_cam_start(&buf[0], C.int(len(buf))) {
	case C.MQ_CAM_OK:
		return nil
	case C.MQ_CAM_DENIED:
		return ErrDenied
	case C.MQ_CAM_PENDING:
		return ErrPending
	default:
		return errors.New(C.GoString(&buf[0]))
	}
}

func stop() { C.mq_cam_stop() }

func setWant(f Feature) { C.mq_cam_want(C.int(f)) }

// grab copies the latest frame into dst (maxW*maxH*3 bytes) and its person mask into mask
// (maxW*maxH bytes).
func grab(dst, mask []byte) (w, h int, seq uint64, hasMask bool, hands []Hand) {
	var cw, ch, hm, nh C.int
	var hs [C.MQ_CAM_HANDS]C.mq_hand
	n := C.mq_cam_latest((*C.uchar)(unsafe.Pointer(&dst[0])), (*C.uchar)(unsafe.Pointer(&mask[0])), &hm, &hs[0], &nh, &cw, &ch)
	for i := range int(nh) {
		c := hs[i]
		hands = append(hands, Hand{
			Index: Point{float64(c.ix), float64(c.iy), float64(c.ic)},
			Thumb: Point{float64(c.tx), float64(c.ty), float64(c.tc)},
			Wrist: Point{float64(c.wx), float64(c.wy), float64(c.wc)},
		})
	}
	return int(cw), int(ch), uint64(n), hm != 0, hands
}

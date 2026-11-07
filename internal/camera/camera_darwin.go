package camera

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework Foundation
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

// grab copies the latest frame into dst (maxW*maxH*3 bytes).
func grab(dst []byte) (w, h int, seq uint64) {
	var cw, ch C.int
	n := C.mq_cam_latest((*C.uchar)(unsafe.Pointer(&dst[0])), &cw, &ch)
	return int(cw), int(ch), uint64(n)
}

package location

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework CoreLocation -framework Foundation
#include "location_darwin.h"
*/
import "C"

import "errors"

func request() { C.mq_loc_request() }

func poll() (Fix, error, bool) {
	var lat, lon C.double
	var name [128]C.char
	var msg [256]C.char
	var out [16]C.char
	switch C.mq_loc_get(&lat, &lon, &name[0], C.int(len(name)), &out[0], C.int(len(out)), &msg[0], C.int(len(msg))) {
	case C.MQ_LOC_OK:
		return Fix{Lat: float64(lat), Lon: float64(lon), Name: C.GoString(&name[0]), Outcode: C.GoString(&out[0])}, nil, true
	case C.MQ_LOC_DENIED:
		return Fix{}, ErrDenied, true
	case C.MQ_LOC_ERROR:
		return Fix{}, errors.New(C.GoString(&msg[0])), true
	}
	return Fix{}, nil, false // not asked yet, or waiting
}

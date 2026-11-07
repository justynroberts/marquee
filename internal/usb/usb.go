// Package usb is a minimal libusb binding: open a device by vendor/product id, claim an
// interface, and do bulk transfers. libusb is linked statically so the binary has no
// runtime dependency on it.
package usb

/*
#cgo darwin CFLAGS: -I/opt/homebrew/include/libusb-1.0 -I/usr/local/include/libusb-1.0
#cgo darwin LDFLAGS: -L/opt/homebrew/lib -L/usr/local/lib -Wl,-search_paths_first /opt/homebrew/lib/libusb-1.0.a -framework IOKit -framework CoreFoundation -framework Security
#cgo linux pkg-config: libusb-1.0
#include <libusb.h>
#include <stdlib.h>

// Find the first bulk IN and bulk OUT endpoint on an interface.
static int find_bulk_eps(libusb_device_handle *h, int iface, unsigned char *in_ep, unsigned char *out_ep) {
	libusb_device *dev = libusb_get_device(h);
	struct libusb_config_descriptor *cfg;
	int rc = libusb_get_active_config_descriptor(dev, &cfg);
	if (rc != 0) return rc;
	*in_ep = 0; *out_ep = 0;
	if (iface < cfg->bNumInterfaces && cfg->interface[iface].num_altsetting > 0) {
		const struct libusb_interface_descriptor *d = &cfg->interface[iface].altsetting[0];
		for (int i = 0; i < d->bNumEndpoints; i++) {
			const struct libusb_endpoint_descriptor *e = &d->endpoint[i];
			if ((e->bmAttributes & 0x3) != LIBUSB_TRANSFER_TYPE_BULK) continue;
			if (e->bEndpointAddress & 0x80) { if (!*in_ep) *in_ep = e->bEndpointAddress; }
			else if (!*out_ep) *out_ep = e->bEndpointAddress;
		}
	}
	libusb_free_config_descriptor(cfg);
	return (*in_ep && *out_ep) ? 0 : LIBUSB_ERROR_NOT_FOUND;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

var (
	initOnce sync.Once
	ctx      *C.libusb_context
	initErr  error
)

func ensureInit() error {
	initOnce.Do(func() {
		if rc := C.libusb_init(&ctx); rc != 0 {
			initErr = libusbError("init", rc)
		}
	})
	return initErr
}

// ErrNotFound means no device with the requested ids is attached.
var ErrNotFound = errors.New("usb: device not found")

func libusbError(op string, rc C.int) error {
	if rc == C.LIBUSB_ERROR_NO_DEVICE {
		return fmt.Errorf("usb %s: %w", op, ErrNotFound)
	}
	return fmt.Errorf("usb %s: %s", op, C.GoString(C.libusb_error_name(rc)))
}

// Device is an opened device with one claimed interface.
type Device struct {
	h      *C.libusb_device_handle
	iface  int
	InEP   uint8
	OutEP  uint8
	closed bool
	mu     sync.Mutex
}

// Open finds vid:pid, claims the interface and discovers its bulk endpoints.
func Open(vid, pid uint16, iface int) (*Device, error) {
	if err := ensureInit(); err != nil {
		return nil, err
	}
	h := C.libusb_open_device_with_vid_pid(ctx, C.uint16_t(vid), C.uint16_t(pid))
	if h == nil {
		return nil, ErrNotFound
	}
	C.libusb_set_auto_detach_kernel_driver(h, 1)
	if rc := C.libusb_claim_interface(h, C.int(iface)); rc != 0 {
		C.libusb_close(h)
		return nil, libusbError("claim interface", rc)
	}
	var in, out C.uchar
	if rc := C.find_bulk_eps(h, C.int(iface), &in, &out); rc != 0 {
		C.libusb_release_interface(h, C.int(iface))
		C.libusb_close(h)
		return nil, libusbError("find endpoints", rc)
	}
	return &Device{h: h, iface: iface, InEP: uint8(in), OutEP: uint8(out)}, nil
}

// Present reports whether a device with these ids is attached, without opening it.
func Present(vid, pid uint16) bool {
	if ensureInit() != nil {
		return false
	}
	var list **C.libusb_device
	n := C.libusb_get_device_list(ctx, &list)
	if n < 0 {
		return false
	}
	defer C.libusb_free_device_list(list, 1)
	devs := unsafe.Slice(list, int(n))
	for _, d := range devs {
		var desc C.struct_libusb_device_descriptor
		if C.libusb_get_device_descriptor(d, &desc) == 0 && uint16(desc.idVendor) == vid && uint16(desc.idProduct) == pid {
			return true
		}
	}
	return false
}

// Write does one bulk OUT transfer and returns the bytes sent.
func (d *Device) Write(data []byte, timeoutMs int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, errors.New("usb: device closed")
	}
	if len(data) == 0 {
		return 0, nil
	}
	var sent C.int
	p := C.CBytes(data)
	defer C.free(p)
	rc := C.libusb_bulk_transfer(d.h, C.uchar(d.OutEP), (*C.uchar)(p), C.int(len(data)), &sent, C.uint(timeoutMs))
	if rc != 0 {
		return int(sent), libusbError("bulk write", rc)
	}
	return int(sent), nil
}

// Read does one bulk IN transfer of up to n bytes.
func (d *Device) Read(n, timeoutMs int) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("usb: device closed")
	}
	buf := C.malloc(C.size_t(n))
	defer C.free(buf)
	var got C.int
	rc := C.libusb_bulk_transfer(d.h, C.uchar(d.InEP), (*C.uchar)(buf), C.int(n), &got, C.uint(timeoutMs))
	if rc != 0 {
		return nil, libusbError("bulk read", rc)
	}
	return C.GoBytes(buf, got), nil
}

// ClearHalt clears a stall on both bulk endpoints.
func (d *Device) ClearHalt() {
	d.mu.Lock()
	defer d.mu.Unlock()
	C.libusb_clear_halt(d.h, C.uchar(d.InEP))
	C.libusb_clear_halt(d.h, C.uchar(d.OutEP))
}

// Reset re-enumerates the device. The handle stays valid unless the device re-attaches
// as a different device, in which case the caller must reopen.
func (d *Device) Reset() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if rc := C.libusb_reset_device(d.h); rc != 0 {
		return libusbError("reset", rc)
	}
	C.libusb_claim_interface(d.h, C.int(d.iface))
	return nil
}

// Close releases the interface and closes the device. Safe to call twice.
func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	C.libusb_release_interface(d.h, C.int(d.iface))
	C.libusb_close(d.h)
}

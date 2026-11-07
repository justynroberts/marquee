// AVFoundation capture of the default camera. The capture queue box-filters each frame down
// to at most MQ_CAM_W pixels wide, mirrors it, and keeps the latest one as RGB; Go copies it
// out with mq_cam_latest. Nothing calls back into Go.
#ifndef MQ_CAMERA_H
#define MQ_CAMERA_H

#define MQ_CAM_W 320
#define MQ_CAM_MAXH 320

enum {
	MQ_CAM_OK = 0,
	MQ_CAM_NODEVICE = 1,
	MQ_CAM_DENIED = 2,
	MQ_CAM_PENDING = 3, // the permission prompt is showing
	MQ_CAM_FAILED = 4,
};

int mq_cam_start(char *err, int errlen);
void mq_cam_stop(void);
// Copies the latest frame (w*h*3 bytes, RGB) into dst, which holds MQ_CAM_W*MQ_CAM_MAXH*3 bytes.
// Returns the frame's sequence number (0 = no frame yet).
long long mq_cam_latest(unsigned char *dst, int *w, int *h);

#endif

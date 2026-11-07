// AVFoundation capture of the default camera. The capture queue box-filters each frame down
// to at most MQ_CAM_W pixels wide, mirrors it, and keeps the latest one as RGB; Go copies it
// out with mq_cam_latest. When asked (mq_cam_want), it also runs Vision person segmentation
// and hand pose on each frame. Nothing calls back into Go.
#ifndef MQ_CAMERA_H
#define MQ_CAMERA_H

#define MQ_CAM_W 320
#define MQ_CAM_MAXH 320
#define MQ_CAM_HANDS 2

enum {
	MQ_CAM_OK = 0,
	MQ_CAM_NODEVICE = 1,
	MQ_CAM_DENIED = 2,
	MQ_CAM_PENDING = 3, // the permission prompt is showing
	MQ_CAM_FAILED = 4,
};

enum {
	MQ_WANT_MASK = 1,
	MQ_WANT_HANDS = 2,
};

// One hand: index tip, thumb tip and wrist, in 0..1 frame coordinates (top-left origin,
// mirrored like the frame), each with Vision's confidence (0 = not seen).
typedef struct {
	float ix, iy, ic;
	float tx, ty, tc;
	float wx, wy, wc;
} mq_hand;

int mq_cam_start(char *err, int errlen);
void mq_cam_stop(void);
// Which Vision analyses to run on coming frames (MQ_WANT_* bits; 0 for none).
void mq_cam_want(int flags);
// Copies the latest frame (w*h*3 bytes, RGB) into dst, which holds MQ_CAM_W*MQ_CAM_MAXH*3
// bytes, and its person mask (w*h bytes, 0..255) into mask when one was computed (*has_mask).
// Returns the frame's sequence number (0 = no frame yet).
long long mq_cam_latest(unsigned char *dst, unsigned char *mask, int *has_mask,
                        mq_hand *hands, int *nhands, int *w, int *h);

#endif

#import <AVFoundation/AVFoundation.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#include <pthread.h>
#include <string.h>
#include "camera_darwin.h"

static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static unsigned char frame[MQ_CAM_W * MQ_CAM_MAXH * 3];
static int fw, fh;
static long long seq;

@interface MQCamDelegate : NSObject <AVCaptureVideoDataOutputSampleBufferDelegate>
@end

@implementation MQCamDelegate
- (void)captureOutput:(AVCaptureOutput *)output
    didOutputSampleBuffer:(CMSampleBufferRef)sb
           fromConnection:(AVCaptureConnection *)conn {
	CVImageBufferRef pb = CMSampleBufferGetImageBuffer(sb);
	if (pb == NULL) return;
	CVPixelBufferLockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
	const unsigned char *base = CVPixelBufferGetBaseAddress(pb);
	size_t w = CVPixelBufferGetWidth(pb), h = CVPixelBufferGetHeight(pb);
	size_t stride = CVPixelBufferGetBytesPerRow(pb);
	if (base == NULL || w == 0 || h == 0) {
		CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
		return;
	}
	int ow = w < MQ_CAM_W ? (int)w : MQ_CAM_W;
	int oh = (int)(h * ow / w);
	if (oh < 1) oh = 1;
	if (oh > MQ_CAM_MAXH) oh = MQ_CAM_MAXH;
	pthread_mutex_lock(&mu);
	for (int oy = 0; oy < oh; oy++) {
		size_t y0 = (size_t)oy * h / oh, y1 = (size_t)(oy + 1) * h / oh;
		if (y1 <= y0) y1 = y0 + 1;
		for (int ox = 0; ox < ow; ox++) {
			size_t x0 = (size_t)ox * w / ow, x1 = (size_t)(ox + 1) * w / ow;
			if (x1 <= x0) x1 = x0 + 1;
			unsigned r = 0, g = 0, b = 0, n = 0;
			for (size_t y = y0; y < y1; y++) {
				const unsigned char *p = base + y * stride + x0 * 4; // BGRA
				for (size_t x = x0; x < x1; x++, p += 4) {
					b += p[0];
					g += p[1];
					r += p[2];
					n++;
				}
			}
			unsigned char *d = frame + ((size_t)oy * ow + (ow - 1 - ox)) * 3; // mirrored, like a selfie view
			d[0] = r / n;
			d[1] = g / n;
			d[2] = b / n;
		}
	}
	fw = ow;
	fh = oh;
	seq++;
	pthread_mutex_unlock(&mu);
	CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
}
@end

static AVCaptureSession *session;
static MQCamDelegate *delegate;
static dispatch_queue_t queue;

static int fail(char *err, int errlen, int code, NSString *msg) {
	if (err != NULL && errlen > 0) {
		strlcpy(err, msg.UTF8String ? msg.UTF8String : "camera error", (size_t)errlen);
	}
	return code;
}

int mq_cam_start(char *err, int errlen) {
	@autoreleasepool {
		if (session != nil) return MQ_CAM_OK;
		switch ([AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo]) {
		case AVAuthorizationStatusAuthorized:
			break;
		case AVAuthorizationStatusNotDetermined:
			[AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo completionHandler:^(BOOL granted){}];
			return fail(err, errlen, MQ_CAM_PENDING, @"waiting for camera permission");
		default:
			return fail(err, errlen, MQ_CAM_DENIED, @"camera access denied");
		}
		AVCaptureDevice *dev = [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo];
		if (dev == nil) return fail(err, errlen, MQ_CAM_NODEVICE, @"no camera");
		NSError *e = nil;
		AVCaptureDeviceInput *in = [AVCaptureDeviceInput deviceInputWithDevice:dev error:&e];
		if (in == nil) return fail(err, errlen, MQ_CAM_FAILED, e.localizedDescription ?: @"camera input failed");
		AVCaptureSession *s = [[AVCaptureSession alloc] init];
		if ([s canSetSessionPreset:AVCaptureSessionPreset640x480]) {
			s.sessionPreset = AVCaptureSessionPreset640x480;
		} else if ([s canSetSessionPreset:AVCaptureSessionPresetMedium]) {
			s.sessionPreset = AVCaptureSessionPresetMedium;
		}
		if (![s canAddInput:in]) return fail(err, errlen, MQ_CAM_FAILED, @"camera input refused");
		[s addInput:in];
		AVCaptureVideoDataOutput *out = [[AVCaptureVideoDataOutput alloc] init];
		out.videoSettings = @{(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_32BGRA)};
		out.alwaysDiscardsLateVideoFrames = YES;
		if (queue == nil) queue = dispatch_queue_create("com.fintonlabs.marquee.camera", DISPATCH_QUEUE_SERIAL);
		if (delegate == nil) delegate = [[MQCamDelegate alloc] init];
		[out setSampleBufferDelegate:delegate queue:queue];
		if (![s canAddOutput:out]) return fail(err, errlen, MQ_CAM_FAILED, @"camera output refused");
		[s addOutput:out];
		[s startRunning];
		if (!s.running) return fail(err, errlen, MQ_CAM_FAILED, @"camera did not start");
		session = s;
		return MQ_CAM_OK;
	}
}

void mq_cam_stop(void) {
	@autoreleasepool {
		AVCaptureSession *s = session;
		session = nil;
		if (s != nil) [s stopRunning];
		pthread_mutex_lock(&mu);
		fw = fh = 0;
		pthread_mutex_unlock(&mu);
	}
}

long long mq_cam_latest(unsigned char *dst, int *w, int *h) {
	pthread_mutex_lock(&mu);
	long long n = seq;
	*w = fw;
	*h = fh;
	if (fw > 0 && fh > 0) memcpy(dst, frame, (size_t)fw * fh * 3);
	pthread_mutex_unlock(&mu);
	return fw > 0 ? n : 0;
}

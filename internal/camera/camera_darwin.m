#import <AVFoundation/AVFoundation.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Vision/Vision.h>
#include <pthread.h>
#include <string.h>
#include "camera_darwin.h"

// Shared with Go, under mu.
static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static unsigned char frame[MQ_CAM_W * MQ_CAM_MAXH * 3];
static unsigned char mask[MQ_CAM_W * MQ_CAM_MAXH];
static int fw, fh, haveMask, nhands;
static mq_hand hands[MQ_CAM_HANDS];
static long long seq;

static volatile int want; // MQ_WANT_* bits

// Scratch used only on the (serial) capture queue.
static unsigned char tframe[MQ_CAM_W * MQ_CAM_MAXH * 3];
static unsigned char tmask[MQ_CAM_W * MQ_CAM_MAXH];
static mq_hand thands[MQ_CAM_HANDS];
static VNGeneratePersonSegmentationRequest *segReq API_AVAILABLE(macos(12.0));
static VNDetectHumanHandPoseRequest *handReq;

static void point(VNHumanHandPoseObservation *o, VNHumanHandPoseObservationJointName j, float *x, float *y, float *c) {
	VNRecognizedPoint *p = [o recognizedPointForJointName:j error:nil];
	if (p == nil || p.confidence <= 0) {
		*x = *y = *c = 0;
		return;
	}
	*x = 1 - p.location.x; // mirrored, like the frame
	*y = 1 - p.location.y; // Vision's origin is bottom-left
	*c = p.confidence;
}

// vision runs the wanted analyses on pb and fills tmask/thands. Returns MQ_WANT_* bits of
// what it produced, and the hand count in *nh.
static int vision(CVPixelBufferRef pb, int w, int ow, int oh, int *nh) {
	*nh = 0;
	if (w == 0) return 0;
	NSMutableArray *reqs = [NSMutableArray array];
	if (w & MQ_WANT_MASK) {
		if (@available(macOS 12.0, *)) {
			if (segReq == nil) {
				segReq = [[VNGeneratePersonSegmentationRequest alloc] init];
				segReq.qualityLevel = VNGeneratePersonSegmentationRequestQualityLevelFast;
				segReq.outputPixelFormat = kCVPixelFormatType_OneComponent8;
			}
			[reqs addObject:segReq];
		}
	}
	if (w & MQ_WANT_HANDS) {
		if (handReq == nil) {
			handReq = [[VNDetectHumanHandPoseRequest alloc] init];
			handReq.maximumHandCount = MQ_CAM_HANDS;
		}
		[reqs addObject:handReq];
	}
	if (reqs.count == 0) return 0;
	VNImageRequestHandler *h = [[VNImageRequestHandler alloc] initWithCVPixelBuffer:pb options:@{}];
	if (![h performRequests:reqs error:nil]) return 0;
	int got = 0;
	if (w & MQ_WANT_MASK) {
		if (@available(macOS 12.0, *)) {
			VNPixelBufferObservation *o = segReq.results.firstObject;
			CVPixelBufferRef m = o.pixelBuffer;
			if (m != NULL) {
				CVPixelBufferLockBaseAddress(m, kCVPixelBufferLock_ReadOnly);
				const unsigned char *mb = CVPixelBufferGetBaseAddress(m);
				size_t mw = CVPixelBufferGetWidth(m), mh = CVPixelBufferGetHeight(m);
				size_t ms = CVPixelBufferGetBytesPerRow(m);
				if (mb != NULL && mw > 0 && mh > 0) {
					for (int oy = 0; oy < oh; oy++) {
						const unsigned char *row = mb + ((size_t)oy * mh / oh) * ms;
						for (int ox = 0; ox < ow; ox++) {
							tmask[oy * ow + (ow - 1 - ox)] = row[(size_t)ox * mw / ow];
						}
					}
					got |= MQ_WANT_MASK;
				}
				CVPixelBufferUnlockBaseAddress(m, kCVPixelBufferLock_ReadOnly);
			}
		}
	}
	if (w & MQ_WANT_HANDS) {
		for (VNHumanHandPoseObservation *o in handReq.results) {
			if (*nh >= MQ_CAM_HANDS) break;
			mq_hand *d = &thands[*nh];
			point(o, VNHumanHandPoseObservationJointNameIndexTip, &d->ix, &d->iy, &d->ic);
			point(o, VNHumanHandPoseObservationJointNameThumbTip, &d->tx, &d->ty, &d->tc);
			point(o, VNHumanHandPoseObservationJointNameWrist, &d->wx, &d->wy, &d->wc);
			(*nh)++;
		}
		got |= MQ_WANT_HANDS;
	}
	return got;
}

@interface MQCamDelegate : NSObject <AVCaptureVideoDataOutputSampleBufferDelegate>
@end

@implementation MQCamDelegate
- (void)captureOutput:(AVCaptureOutput *)output
    didOutputSampleBuffer:(CMSampleBufferRef)sb
           fromConnection:(AVCaptureConnection *)conn {
	@autoreleasepool {
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
				unsigned char *d = tframe + ((size_t)oy * ow + (ow - 1 - ox)) * 3; // mirrored, like a selfie view
				d[0] = r / n;
				d[1] = g / n;
				d[2] = b / n;
			}
		}
		CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);

		// Vision at most 15 times a second: the panel shows 12-20 frames a second, so running it
		// on every 30 fps camera frame only burns CPU. Between runs the last results stand.
		static CFAbsoluteTime lastVision;
		static int lastGot, lastNh;
		int nh = lastNh;
		int got = lastGot;
		CFAbsoluteTime now = CFAbsoluteTimeGetCurrent();
		int wanted = want;
		if (wanted == 0) {
			got = nh = lastGot = lastNh = 0;
		} else if (now - lastVision >= 1.0 / 15 || (got & wanted) != wanted) {
			lastVision = now;
			got = vision(pb, wanted, ow, oh, &nh);
			lastGot = got;
			lastNh = nh;
		}

		pthread_mutex_lock(&mu);
		memcpy(frame, tframe, (size_t)ow * oh * 3);
		haveMask = (got & MQ_WANT_MASK) != 0;
		if (haveMask) memcpy(mask, tmask, (size_t)ow * oh);
		nhands = nh;
		memcpy(hands, thands, sizeof(mq_hand) * (size_t)nh);
		fw = ow;
		fh = oh;
		seq++;
		pthread_mutex_unlock(&mu);
	}
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
		want = 0;
		pthread_mutex_lock(&mu);
		fw = fh = 0;
		haveMask = nhands = 0;
		pthread_mutex_unlock(&mu);
	}
}

void mq_cam_want(int flags) { want = flags; }

long long mq_cam_latest(unsigned char *dst, unsigned char *mdst, int *has_mask,
                        mq_hand *hdst, int *nh, int *w, int *h) {
	pthread_mutex_lock(&mu);
	long long n = seq;
	*w = fw;
	*h = fh;
	*has_mask = haveMask;
	*nh = nhands;
	if (fw > 0 && fh > 0) {
		memcpy(dst, frame, (size_t)fw * fh * 3);
		if (haveMask) memcpy(mdst, mask, (size_t)fw * fh);
	}
	memcpy(hdst, hands, sizeof(mq_hand) * (size_t)nhands);
	pthread_mutex_unlock(&mu);
	return fw > 0 ? n : 0;
}

#import <CoreLocation/CoreLocation.h>
#import <Foundation/Foundation.h>
#include <pthread.h>
#include <string.h>
#include "location_darwin.h"

// CLGeocoder is deprecated from macOS 26 in favour of MapKit, whose replacement needs 26; the
// app supports macOS 12.
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static int state = MQ_LOC_NONE;
static double gLat, gLon;
static char gName[128];
static char gOutcode[16];
static char gErr[256];

static void setState(int s, const char *err) {
	pthread_mutex_lock(&mu);
	state = s;
	if (err) strlcpy(gErr, err, sizeof gErr);
	pthread_mutex_unlock(&mu);
}

@interface MQLocation : NSObject <CLLocationManagerDelegate>
@property(strong) CLLocationManager *manager;
@property(strong) CLGeocoder *geocoder;
- (void)ask;
@end

@implementation MQLocation
- (void)ask {
	CLAuthorizationStatus s = self.manager.authorizationStatus;
	if (s == kCLAuthorizationStatusNotDetermined) {
		setState(MQ_LOC_PENDING, NULL);
		[self.manager requestWhenInUseAuthorization];
	} else if (s == kCLAuthorizationStatusDenied || s == kCLAuthorizationStatusRestricted) {
		setState(MQ_LOC_DENIED, "location access is off for Marquee");
	} else {
		[self.manager requestLocation];
	}
}

- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)m {
	if (m.authorizationStatus != kCLAuthorizationStatusNotDetermined) [self ask];
}

- (void)locationManager:(CLLocationManager *)m didUpdateLocations:(NSArray<CLLocation *> *)locations {
	CLLocation *l = locations.lastObject;
	if (!l) return;
	pthread_mutex_lock(&mu);
	gLat = l.coordinate.latitude;
	gLon = l.coordinate.longitude;
	pthread_mutex_unlock(&mu);
	[self.geocoder reverseGeocodeLocation:l completionHandler:^(NSArray<CLPlacemark *> *marks, NSError *error) {
		CLPlacemark *p = marks.firstObject;
		NSString *n = p.locality ?: p.subAdministrativeArea ?: p.administrativeArea ?: p.name ?: @"";
		// keep only the postcode's district ("SW1A"), never the full postcode
		NSString *pc = [p.postalCode componentsSeparatedByString:@" "].firstObject ?: @"";
		pthread_mutex_lock(&mu);
		strlcpy(gName, n.UTF8String ?: "", sizeof gName);
		strlcpy(gOutcode, pc.UTF8String ?: "", sizeof gOutcode);
		state = MQ_LOC_OK;
		pthread_mutex_unlock(&mu);
	}];
}

- (void)locationManager:(CLLocationManager *)m didFailWithError:(NSError *)error {
	if (error.code == kCLErrorLocationUnknown) return; // keeps trying
	if (error.code == kCLErrorDenied) {
		setState(MQ_LOC_DENIED, "location access is off for Marquee");
		return;
	}
	setState(MQ_LOC_ERROR, error.localizedDescription.UTF8String);
}
@end

static MQLocation *loc;

void mq_loc_request(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!loc) {
			loc = [MQLocation new];
			loc.manager = [CLLocationManager new];
			loc.manager.desiredAccuracy = kCLLocationAccuracyKilometer;
			loc.manager.delegate = loc;
			loc.geocoder = [CLGeocoder new];
		}
		[loc ask];
	});
}

int mq_loc_get(double *lat, double *lon, char *name, int nameLen, char *outcode, int outLen, char *err, int errLen) {
	pthread_mutex_lock(&mu);
	int s = state;
	strlcpy(outcode, gOutcode, outLen);
	*lat = gLat;
	*lon = gLon;
	strlcpy(name, gName, nameLen);
	strlcpy(err, gErr, errLen);
	pthread_mutex_unlock(&mu);
	return s;
}

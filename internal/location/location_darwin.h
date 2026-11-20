// Core Location bridge: one coarse fix for this Mac, named by the reverse geocoder.

enum { MQ_LOC_NONE = 0, MQ_LOC_PENDING = 1, MQ_LOC_DENIED = 2, MQ_LOC_OK = 3, MQ_LOC_ERROR = 4 };

void mq_loc_request(void);
int mq_loc_get(double *lat, double *lon, char *name, int nameLen, char *outcode, int outLen, char *err, int errLen);

package queue

const (
	schemePush        = "push"
	schemeCloudTasks  = "cloudtasks"
	schemeCEHTTP      = "ce+http"
	schemeCEHTTPS     = "ce+https"
	defaultMaxBodyMiB = 20 // 1 << 20 = 1 MiB
)

package queue

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// DeliveryMode classifies how a subscriber receives messages.
type DeliveryMode int

const (
	// DeliveryModePull opens a gocloud subscription and runs a receive loop.
	DeliveryModePull DeliveryMode = iota
	// DeliveryModePush receives messages via the HTTP multiplexing handler.
	DeliveryModePush
)

func (m DeliveryMode) String() string {
	switch m {
	case DeliveryModePull:
		return "pull"
	case DeliveryModePush:
		return schemePush
	default:
		return fmt.Sprintf("DeliveryMode(%d)", int(m))
	}
}

// PublishKind classifies how a publisher sends messages.
type PublishKind int

const (
	// PublishKindGoCloud uses gocloud OpenTopic (mem://, nats://, gcppubsub://).
	PublishKindGoCloud PublishKind = iota
	// PublishKindCloudEventsHTTP POSTs CloudEvents binary mode over HTTP.
	PublishKindCloudEventsHTTP
	// PublishKindCloudTasks creates Cloud Tasks via the REST API.
	PublishKindCloudTasks
)

func (k PublishKind) String() string {
	switch k {
	case PublishKindGoCloud:
		return "gocloud"
	case PublishKindCloudEventsHTTP:
		return "cloudevents"
	case PublishKindCloudTasks:
		return schemeCloudTasks
	default:
		return fmt.Sprintf("PublishKind(%d)", int(k))
	}
}

// ClassifySubscriberURL parses queueURL and returns DeliveryMode.
// Uses net/url.Parse — not strings.HasPrefix.
func ClassifySubscriberURL(queueURL string) (DeliveryMode, url.Values, error) {
	u, err := url.Parse(queueURL)
	if err != nil || u.Scheme == "" {
		if err == nil {
			err = errors.New("missing scheme")
		}
		return 0, nil, fmt.Errorf("queue: invalid subscriber URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "mem", "nats", schemeGCPPubSub:
		// gcppubsub:// is Go Cloud pull (StreamingPull). A GCP-side push
		// subscription that POSTs to this service uses push:// instead.
		return DeliveryModePull, u.Query(), nil
	case schemePush, "http", "https":
		return DeliveryModePush, u.Query(), nil
	default:
		return 0, nil, fmt.Errorf("queue: unsupported subscriber scheme %q", u.Scheme)
	}
}

// ClassifyPublisherURL parses queueURL and returns PublishKind.
func ClassifyPublisherURL(queueURL string) (PublishKind, error) {
	u, err := url.Parse(queueURL)
	if err != nil || u.Scheme == "" {
		if err == nil {
			err = errors.New("missing scheme")
		}
		return 0, fmt.Errorf("queue: invalid publisher URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "mem", "nats", schemeGCPPubSub:
		return PublishKindGoCloud, nil
	case schemeCEHTTP, schemeCEHTTPS:
		return PublishKindCloudEventsHTTP, nil
	case schemeCloudTasks:
		return PublishKindCloudTasks, nil
	default:
		return 0, fmt.Errorf("queue: unsupported publisher scheme %q", u.Scheme)
	}
}

// CloudEventsRequestURL strips ce+http(s) → http(s) and returns Frame config query params.
// Config query keys (type, source, …) are not forwarded on the wire URL.
func CloudEventsRequestURL(raw string) (string, url.Values, error) {
	u, parseErr := url.Parse(raw)
	if parseErr != nil {
		return "", nil, parseErr
	}
	switch strings.ToLower(u.Scheme) {
	case schemeCEHTTPS:
		u.Scheme = "https"
	case schemeCEHTTP:
		u.Scheme = "http"
	default:
		return "", nil, errors.New("queue: not a ce+http(s) URL")
	}
	q := u.Query()
	u.RawQuery = ""
	return u.String(), q, nil
}

// CloudTasksTarget is a parsed cloudtasks:// publisher URL.
type CloudTasksTarget struct {
	Project       string
	Location      string
	Queue         string
	ResourceName  string // projects/p/locations/l/queues/q (no leading slash)
	TargetURL     string
	OIDCSA        string
	OIDCAudience  string
	ScheduleDelay time.Duration
	TaskID        string
	// DeprecatedHostForm is true when the URL used host=projects (ambiguous form).
	DeprecatedHostForm bool
}

// ParseCloudTasksURL parses the canonical and accepted legacy Cloud Tasks URL forms.
//
// Canonical:
//
//	cloudtasks:///projects/{p}/locations/{l}/queues/{q}?url=...
func ParseCloudTasksURL(raw string) (*CloudTasksTarget, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("queue: cloudtasks URL parse: %w", err)
	}
	if strings.ToLower(u.Scheme) != schemeCloudTasks {
		return nil, errors.New("queue: not a cloudtasks URL")
	}

	var resourcePath string
	deprecated := false
	q := u.Query()

	switch {
	case u.Host == "" && strings.HasPrefix(u.Path, "/projects/"):
		resourcePath = strings.TrimPrefix(u.Path, "/")
	case u.Host == "projects" && u.Path != "":
		// Ambiguous form cloudtasks://projects/p/locations/... — support once.
		deprecated = true
		resourcePath = path.Join(u.Host, strings.TrimPrefix(u.Path, "/"))
	case q.Get("project") != "" && q.Get("location") != "" && q.Get("queue") != "":
		resourcePath = fmt.Sprintf("projects/%s/locations/%s/queues/%s",
			q.Get("project"), q.Get("location"), q.Get("queue"))
	default:
		return nil, errors.New(
			"queue: cloudtasks URL must be cloudtasks:///projects/{p}/locations/{l}/queues/{q}?url=",
		)
	}

	parts := strings.Split(resourcePath, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "queues" {
		return nil, fmt.Errorf("queue: invalid cloudtasks resource path %q", resourcePath)
	}
	if parts[1] == "" || parts[3] == "" || parts[5] == "" {
		return nil, fmt.Errorf("queue: empty project/location/queue in %q", resourcePath)
	}

	target := q.Get("url")
	if target == "" {
		return nil, errors.New("queue: cloudtasks URL missing required query param url")
	}
	if _, uriErr := url.ParseRequestURI(target); uriErr != nil {
		return nil, fmt.Errorf("queue: cloudtasks url param invalid: %w", uriErr)
	}

	ct := &CloudTasksTarget{
		Project:            parts[1],
		Location:           parts[3],
		Queue:              parts[5],
		ResourceName:       resourcePath,
		TargetURL:          target,
		OIDCSA:             q.Get("oidc_sa"),
		OIDCAudience:       q.Get("oidc_audience"),
		TaskID:             q.Get("task_id"),
		DeprecatedHostForm: deprecated,
	}

	if d := q.Get("schedule_delay"); d != "" {
		dur, durErr := time.ParseDuration(d)
		if durErr != nil {
			// also accept plain seconds
			if secs, secErr := strconv.ParseInt(d, 10, 64); secErr == nil {
				dur = time.Duration(secs) * time.Second
			} else {
				return nil, fmt.Errorf("queue: cloudtasks schedule_delay invalid: %w", durErr)
			}
		}
		ct.ScheduleDelay = dur
	}

	return ct, nil
}

// CreateTaskAPIURL returns the Cloud Tasks v2 CreateTask REST endpoint.
func (t *CloudTasksTarget) CreateTaskAPIURL() string {
	return "https://cloudtasks.googleapis.com/v2/" + t.ResourceName + "/tasks"
}

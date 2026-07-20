package protocol

import (
	"io"
	"net/http"
	"strings"
)

// Cloud Tasks HTTP target headers (canonical MIME form used by net/http.Header.Get).
const (
	HeaderTaskName       = "X-Cloudtasks-Taskname"
	HeaderQueueName      = "X-Cloudtasks-Queuename"
	HeaderTaskRetryCount = "X-Cloudtasks-Taskretrycount"
	HeaderTaskExecCount  = "X-Cloudtasks-Taskexecutioncount"
	HeaderTaskETA        = "X-Cloudtasks-Tasketa"
)

// CloudTasks matches Cloud Tasks push requests and maps headers into metadata.
type CloudTasks struct{}

func (CloudTasks) Name() string { return ProtocolCloudTasks }

func (CloudTasks) Match(r *http.Request) bool {
	return r.Header.Get(HeaderTaskName) != "" || r.Header.Get(HeaderQueueName) != ""
}

func (CloudTasks) Decode(r *http.Request) (*Inbound, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	md := make(map[string]string)
	copyHeader(md, r, HeaderTaskName, "cloudtasks.task_name")
	copyHeader(md, r, HeaderQueueName, "cloudtasks.queue_name")
	copyHeader(md, r, HeaderTaskRetryCount, "cloudtasks.retry_count")
	copyHeader(md, r, HeaderTaskExecCount, "cloudtasks.execution_count")
	copyHeader(md, r, HeaderTaskETA, "cloudtasks.eta")

	// Also pull CE attributes if present on the same request (Tasks wrapping CE)
	for k, vals := range r.Header {
		if len(vals) == 0 {
			continue
		}
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "ce-") {
			md[lk] = vals[0]
			if lk == metaCEFrameEvent {
				md[metaFrameEvent] = vals[0]
			}
			continue
		}
		switch lk {
		case metaTraceParent, metaTraceState, metaBaggage, metaContentType:
			md[lk] = vals[0]
		}
	}
	return &Inbound{Body: body, Metadata: md}, nil
}

func copyHeader(md map[string]string, r *http.Request, header, metaKey string) {
	if v := r.Header.Get(header); v != "" {
		md[metaKey] = v
	}
}

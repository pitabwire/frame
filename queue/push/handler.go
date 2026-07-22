package push

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v2/queue"
	"github.com/pitabwire/frame/v2/queue/protocol"
)

// DefaultBasePath is the reserved mux path for queue push delivery.
const DefaultBasePath = "/_frame/queue"

const (
	defaultMaxBodyShift = 20 // 1 << 20 = 1 MiB
)

// Config configures the multiplexing push handler.
type Config struct {
	BasePath     string
	Auth         Authenticator
	TrustClaims  bool
	AckPoison    bool
	MaxBodyBytes int64
	// HandlerTimeout bounds the delivery context. Zero means no timeout —
	// queue consumers process until the handler returns (background work).
	HandlerTimeout time.Duration
	// ProtocolOverride forces a codec; empty uses subscriber URL query or auto.
	ProtocolOverride string
}

// Handler demultiplexes POST /{base}/{ref} to registered push subscribers.
type Handler struct {
	lookup queue.PushLookup
	cfg    Config
	codecs []protocol.Codec
}

// NewHandler builds a push multiplexing handler.
func NewHandler(lookup queue.PushLookup, cfg Config) *Handler {
	if cfg.BasePath == "" {
		cfg.BasePath = DefaultBasePath
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << defaultMaxBodyShift
	}
	// HandlerTimeout <= 0 is intentional: requestContext leaves the context
	// unbounded so long-running event handlers are not killed mid-flight.
	if cfg.Auth == nil {
		cfg.Auth = NoneAuth{}
	}
	return &Handler{
		lookup: lookup,
		cfg:    cfg,
		codecs: protocol.DefaultCodecs(),
	}
}

// Register mounts the handler on mux with a methodless path pattern.
func (h *Handler) Register(mux *http.ServeMux) {
	p := strings.TrimRight(h.cfg.BasePath, "/") + "/{ref}"
	mux.Handle(p, h)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ref := r.PathValue("ref")
	if ref == "" {
		http.NotFound(w, r)
		return
	}

	start := time.Now()
	log := util.Log(r.Context()).
		WithField("queue_ref", ref).
		WithField("delivery_mode", "push")

	if err := h.cfg.Auth.Authenticate(r); err != nil {
		status := HTTPStatusFor(err)
		log.WithError(err).WithField("http_status", status).Warn("push auth failed")
		http.Error(w, http.StatusText(status), status)
		return
	}

	target, ok := h.lookup.LookupPush(ref)
	if !ok {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes)

	protocolPref := h.cfg.ProtocolOverride
	if protocolPref == "" {
		protocolPref = protocolFromSubscriberURL(h.lookup, ref)
	}

	codec := protocol.SelectCodec(protocolPref, r, h.codecs)
	inbound, err := codec.Decode(r)
	if err != nil {
		h.writeDecodeError(w, log, err)
		return
	}

	md := protocol.SanitizeInbound(inbound.Metadata, h.cfg.TrustClaims)
	ctx, cancel := h.requestContext(r)
	if cancel != nil {
		defer cancel()
	}

	procErr := target.ProcessPush(ctx, md, inbound.Body)
	h.writeProcessResult(w, log, codec.Name(), start, procErr)
}

func (h *Handler) requestContext(r *http.Request) (context.Context, context.CancelFunc) {
	if h.cfg.HandlerTimeout <= 0 {
		return r.Context(), nil
	}
	return context.WithTimeout(r.Context(), h.cfg.HandlerTimeout)
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, log *util.LogEntry, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) || isTooLarge(err) {
		log.WithError(err).WithField("http_status", http.StatusRequestEntityTooLarge).Warn("push body too large")
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}
	log.WithError(err).WithField("http_status", http.StatusBadRequest).Warn("push decode failed")
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
}

func (h *Handler) writeProcessResult(
	w http.ResponseWriter,
	log *util.LogEntry,
	protocolName string,
	start time.Time,
	procErr error,
) {
	status := HTTPStatusFor(procErr)
	if procErr != nil && errors.Is(procErr, queue.ErrNotRetryable) && h.cfg.AckPoison {
		status = http.StatusOK
		log.WithError(procErr).WithField("http_status", status).Warn("push poison acked")
		w.WriteHeader(status)
		return
	}
	if procErr != nil {
		log.WithError(procErr).
			WithField("http_status", status).
			WithField("protocol", protocolName).
			WithField("duration_ms", time.Since(start).Milliseconds()).
			Warn("push handler error")
		http.Error(w, http.StatusText(status), status)
		return
	}

	log.WithField("http_status", status).
		WithField("protocol", protocolName).
		WithField("duration_ms", time.Since(start).Milliseconds()).
		Debug("push delivered")
	w.WriteHeader(status)
}

func protocolFromSubscriberURL(m queue.PushLookup, ref string) string {
	sub, err := m.GetSubscriber(ref)
	if err != nil {
		return protocol.ProtocolAuto
	}
	u, err := url.Parse(sub.URI())
	if err != nil {
		return protocol.ProtocolAuto
	}
	p := u.Query().Get("protocol")
	if p == "" {
		return protocol.ProtocolAuto
	}
	return p
}

func isTooLarge(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, queue.ErrTooLarge) {
		return true
	}
	return strings.Contains(err.Error(), "http: request body too large")
}

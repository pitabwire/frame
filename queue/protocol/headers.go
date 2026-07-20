package protocol

// Shared header / metadata key constants (lowercase wire forms after normalize).
const (
	metaTraceParent  = "traceparent"
	metaTraceState   = "tracestate"
	metaBaggage      = "baggage"
	metaContentType  = "content-type"
	metaLang         = "lang"
	metaFrameEvent   = "frame._internal.event.header"
	metaCEFrameEvent = "ce-frameevent"
)

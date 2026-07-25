package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/pubsub"

	"github.com/pitabwire/frame/v2/localization"
	"github.com/pitabwire/frame/v2/security"
	"github.com/pitabwire/frame/v2/workerpool"
)

// processDelivery runs context enrichment and all handlers for one message.
// It does not touch Ack/Nack or ActiveMessages metrics.
func (s *subscriber) processDelivery(ctx context.Context, metadata map[string]string, body []byte) error {
	if metadata == nil {
		metadata = map[string]string{}
	}

	// Reconstruct auth claims from publisher metadata so storage-layer
	// tenancy (RLS) can filter on the published tenant/partitions.
	// Do not blanket-call SkipTenancyChecksOnClaims: that flag maps to
	// tenancy.Claims.Skip and would disable RLS for every consumer.
	// Internal/system roles still skip via ClaimsToContext / IsInternalSystem.
	pCtx := ctx
	authClaim := security.ClaimsFromMap(metadata)
	if authClaim != nil {
		pCtx = authClaim.ClaimsToContext(pCtx)
		pCtx = util.SetTenancy(pCtx, authClaim)
	}

	// Extract remote span context for linking, not parenting.
	var carrier propagation.MapCarrier = metadata
	extractedCtx := otel.GetTextMapPropagator().Extract(pCtx, carrier)
	remoteSpanCtx := trace.SpanContextFromContext(extractedCtx)

	var spanOpts []trace.SpanStartOption
	spanOpts = append(spanOpts, trace.WithNewRoot())
	if remoteSpanCtx.IsValid() {
		spanOpts = append(spanOpts, trace.WithLinks(trace.Link{SpanContext: remoteSpanCtx}))
	}

	var err error
	pCtx, span := s.tracer.Start(pCtx, "process", spanOpts...)
	defer func() { s.tracer.End(pCtx, span, err) }()

	languages := localization.FromMap(metadata)
	if len(languages) > 0 {
		pCtx = localization.ToContext(pCtx, languages)
	}

	for _, worker := range s.handlers {
		err = worker.Handle(pCtx, metadata, body)
		if err != nil {
			util.Log(pCtx).
				WithField("name", s.reference).
				WithField("function", "processDelivery").
				WithField("url", s.url).
				WithError(err).
				Warn("could not handle message")
			return err
		}
	}
	return nil
}

// processReceivedMessage is the pull-path entry: submit job, Ack/Nack inside.
func (s *subscriber) processReceivedMessage(ctx context.Context, msg *pubsub.Message) error {
	job := workerpool.NewJob[any](func(jobCtx context.Context, _ workerpool.JobResultPipe[any]) error {
		var err error
		defer s.metrics.closeMessage(time.Now(), err)

		err = s.processDelivery(jobCtx, msg.Metadata, msg.Body)
		if err != nil {
			msg.Nack()
			return err
		}
		msg.Ack()
		return nil
	})

	submitErr := workerpool.SubmitJob[any](ctx, s.workManager, job)
	if submitErr != nil {
		msg.Nack()
		logger := util.Log(ctx).
			WithField("name", s.reference).
			WithField("function", "processReceivedMessage").
			WithField("url", s.url)
		logger.WithError(submitErr).Error("could not process message, failed to submit job")
		s.metrics.closeMessage(time.Now(), submitErr)
		return submitErr
	}

	return nil
}

// processDeliverySync runs processDelivery on the worker pool and waits for completion.
// Used by the HTTP push handler so status can be returned on the same request.
func (s *subscriber) processDeliverySync(ctx context.Context, metadata map[string]string, body []byte) error {
	s.storeState(SubscriberStateProcessing)
	s.metrics.LastActivity.Store(time.Now().UnixNano())
	s.metrics.ActiveMessages.Add(1)

	defer func() {
		s.storeState(SubscriberStateWaiting)
	}()

	done := make(chan error, 1)
	job := workerpool.NewJobWithRetry[any](
		func(jobCtx context.Context, _ workerpool.JobResultPipe[any]) error {
			var jobErr error
			defer func() {
				if rec := recover(); rec != nil {
					jobErr = fmt.Errorf("queue: handler panic: %v", rec)
				}
				select {
				case done <- jobErr:
				default:
				}
			}()
			jobErr = s.processDelivery(jobCtx, metadata, body)
			return jobErr
		},
		0,
	) // retries explicitly 0 — one HTTP request must not multi-execute

	if submitErr := workerpool.SubmitJob[any](ctx, s.workManager, job); submitErr != nil {
		s.metrics.closeMessage(time.Now(), submitErr)
		return submitErr
	}

	select {
	case procErr := <-done:
		s.metrics.closeMessage(time.Now(), procErr)
		return procErr
	case <-ctx.Done():
		s.metrics.closeMessage(time.Now(), ctx.Err())
		return ctx.Err()
	}
}

// ProcessPush implements PushTarget for HTTP push delivery.
func (s *subscriber) ProcessPush(ctx context.Context, metadata map[string]string, body []byte) error {
	return s.processDeliverySync(ctx, metadata, body)
}

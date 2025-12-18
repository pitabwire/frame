package queue

import (
	"context"
	"maps"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"gocloud.dev/pubsub"

	"github.com/pitabwire/frame/internal"
	"github.com/pitabwire/frame/localization"
	"github.com/pitabwire/frame/security"
)

type publisher struct {
	reference string
	url       string
	topic     *pubsub.Topic
	isInit    atomic.Bool
}

func newPublisher(reference string, queueURL string) Publisher {
	return &publisher{
		reference: reference,
		url:       queueURL,
	}
}

func (p *publisher) Ref() string {
	return p.reference
}

func (p *publisher) Publish(ctx context.Context, payload any, headers ...map[string]string) error {
	metadata := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, metadata)

	for _, h := range headers {
		maps.Copy(metadata, h)
	}

	authClaim := security.ClaimsFromContext(ctx)
	if authClaim != nil {
		maps.Copy(metadata, authClaim.AsMetadata())
	}

	language := localization.FromContext(ctx)
	if len(language) > 0 {
		metadata = localization.ToMap(metadata, language)
	}

	metadata = security.SkipTenancyChecksToMap(ctx, metadata)

	message, err := internal.Marshal(payload)
	if err != nil {
		return err
	}

	topic := p.topic

	return topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})
}

func (p *publisher) Init(ctx context.Context) error {
	if p.isInit.Load() && p.topic != nil {
		return nil
	}

	var err error

	p.topic, err = pubsub.OpenTopic(ctx, p.url)
	if err != nil {
		return err
	}

	p.isInit.Store(true)
	return nil
}

func (p *publisher) Initiated() bool {
	return p.isInit.Load()
}

const defaultPublisherShutdownTimeoutSeconds = 30

func (p *publisher) Stop(ctx context.Context) error {
	// TODO: incooporate trace information in shutdown context
	var sctx context.Context
	var cancelFunc context.CancelFunc

	select {
	case <-ctx.Done():
		sctx = context.Background()
	default:
		sctx = ctx
	}

	sctx, cancelFunc = context.WithTimeout(sctx, time.Second*defaultPublisherShutdownTimeoutSeconds)
	defer cancelFunc()

	p.isInit.Store(false)

	err := p.topic.Shutdown(sctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *publisher) As(i any) bool {
	return p.topic.As(i)
}

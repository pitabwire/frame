package queue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"gocloud.dev/pubsub"
	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/v2/internal"
	"github.com/pitabwire/frame/v2/localization"
	"github.com/pitabwire/frame/v2/security"
)

const responseDrainLimit = 1 << defaultMaxBodyMiB

type publisher struct {
	reference string
	url       string
	kind      PublishKind

	topic  *pubsub.Topic
	isInit atomic.Bool

	httpClient  *http.Client
	tokenSource oauth2.TokenSource
	serviceName string

	ceConfig *cloudEventsPublishConfig
	ctTarget *CloudTasksTarget
}

func newPublisher(reference string, queueURL string) *publisher {
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

	message, err := internal.Marshal(payload)
	if err != nil {
		return err
	}

	switch p.kind {
	case PublishKindGoCloud:
		return p.publishGoCloud(ctx, message, metadata)
	case PublishKindCloudEventsHTTP:
		return p.publishCloudEvents(ctx, message, metadata)
	case PublishKindCloudTasks:
		return p.publishCloudTasks(ctx, message, metadata)
	default:
		return fmt.Errorf("queue: unknown publish kind %v", p.kind)
	}
}

func (p *publisher) publishGoCloud(ctx context.Context, message []byte, metadata map[string]string) error {
	topic := p.topic
	if topic == nil {
		return errors.New("publisher is not initialized")
	}
	return topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})
}

func (p *publisher) Init(ctx context.Context) error {
	if p.alreadyInit() {
		return nil
	}

	kind, err := ClassifyPublisherURL(p.url)
	if err != nil {
		return err
	}
	p.kind = kind

	if initErr := p.initByKind(ctx, kind); initErr != nil {
		return initErr
	}

	p.isInit.Store(true)
	return nil
}

func (p *publisher) alreadyInit() bool {
	if !p.isInit.Load() {
		return false
	}
	if p.kind == PublishKindGoCloud {
		return p.topic != nil
	}
	return true
}

func (p *publisher) initByKind(ctx context.Context, kind PublishKind) error {
	switch kind {
	case PublishKindGoCloud:
		topic, err := pubsub.OpenTopic(ctx, p.url)
		if err != nil {
			return err
		}
		p.topic = topic
		return nil
	case PublishKindCloudEventsHTTP:
		return p.initCloudEvents()
	case PublishKindCloudTasks:
		return p.initCloudTasks()
	default:
		return fmt.Errorf("queue: unknown publish kind %v", kind)
	}
}

func (p *publisher) initCloudEvents() error {
	cfg, err := parseCloudEventsPublishConfig(p.url, p.reference, p.serviceName)
	if err != nil {
		return err
	}
	if p.httpClient == nil {
		return fmt.Errorf("publisher %s: cloudevents: HTTP client not configured", p.reference)
	}
	p.ceConfig = cfg
	return nil
}

func (p *publisher) initCloudTasks() error {
	target, err := ParseCloudTasksURL(p.url)
	if err != nil {
		return err
	}
	if p.tokenSource == nil {
		return fmt.Errorf("publisher %s: cloudtasks: no credentials configured", p.reference)
	}
	if p.httpClient == nil {
		return fmt.Errorf("publisher %s: cloudtasks: HTTP client not configured", p.reference)
	}
	p.ctTarget = target
	return nil
}

func (p *publisher) Initiated() bool {
	return p.isInit.Load()
}

const defaultPublisherShutdownTimeoutSeconds = 30

func (p *publisher) Stop(ctx context.Context) error {
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

	if p.topic == nil {
		return nil
	}

	if strings.HasPrefix(strings.ToLower(p.url), "mem://") {
		p.topic = nil
		return nil
	}

	err := p.topic.Shutdown(sctx)
	if err != nil {
		if isTopicAlreadyShutdownErr(err) {
			p.topic = nil
			return nil
		}
		return err
	}

	p.topic = nil
	return nil
}

func (p *publisher) As(i any) bool {
	if p.topic == nil {
		return false
	}
	return p.topic.As(i)
}

func isTopicAlreadyShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "topic has been shutdown")
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, responseDrainLimit))
	_ = resp.Body.Close()
}

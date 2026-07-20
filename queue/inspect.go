package queue

type PublisherInfo struct {
	Reference string `json:"reference"`
	URL       string `json:"url"`
	Initiated bool   `json:"initiated"`
}

type SubscriberInfo struct {
	Reference string          `json:"reference"`
	URL       string          `json:"url"`
	State     SubscriberState `json:"state"`
	Initiated bool            `json:"initiated"`
	Mode      DeliveryMode    `json:"mode"`
}

type Inspector interface {
	ListPublishers() []PublisherInfo
	ListSubscribers() []SubscriberInfo
}

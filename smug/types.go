// shared types across smug

package smug

import "time"

type ContentType int

const (
	CONTENT_DISPLAY = iota
	CONTENT_META    = iota
)

func (c ContentType) String() string {
	return [...]string{"Display", "Meta"}[c]
}

type Broker interface {
	Name() string
	// called for every event
	HandleEvent(*Event, Dispatcher)
	// after Setup(), the broker should be able to Handle(event) as needed.
	// may require a queue until Activate() is called by dispatcher.AddBroker
	Setup(...string)
	// this will setup a runloop if needed for the broker
	Activate(Dispatcher)
	// called during destruction
	Deactivate()
	// if true not returned, broker assumed to be dead.
	// should cause broker to output a logline with metrics
	Heartbeat() bool
}

type Dispatcher interface {
	Broadcast(*Event)
	AddBroker(Broker)
	RemoveBroker(Broker) error
	NumBrokers() int
	Heartbeat()
}

type EventBlock struct {
	// some event displays could use a bit more layout control
	Title  string
	Text   string
	ImgUrl string
	Type   ContentType
}

type Event struct {
	IsCmdOutput bool
	Origin      Broker
	ReplyBroker Broker // all brokers will see message but may choose to ignore
	// unless beneficial (bot handlers, etc)
	ReplyTarget string // replyBroker will use this to target a specific user
	// either privately or some other mechanism. this should
	// not be changed once set by the originating event as it
	// may specific to a given broker's format
	Actor         string
	Avatar        string
	Text          string
	RawText       string
	ContentBlocks []*EventBlock
	ts            time.Time
}

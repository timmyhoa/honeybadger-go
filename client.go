package honeybadger

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// The Payload interface is implemented by any type which can be handled by the
// Backend interface.
type Payload interface {
	toJSON() []byte
}

// The Backend interface is implemented by the server type by default, but a
// custom implementation may be configured by the user.
type Backend interface {
	Notify(feature Feature, payload Payload) error
}

type noticeHandler func(*Notice) error

// Client is the manager for interacting with the Honeybadger service. It holds
// the configuration and implements the public API.
type Client struct {
	Config               *Configuration
	context              *contextSync
	worker               worker
	beforeNotifyHandlers []noticeHandler
}

// Configure updates the client configuration with the supplied config.
func (client *Client) Configure(config Configuration) {
	client.Config.update(&config)
}

// SetContext updates the client context with supplied context.
func (client *Client) SetContext(context Context) {
	client.context.Update(context)
}

// Flush blocks until the worker has processed its queue.
func (client *Client) Flush() {
	client.worker.Flush()
}

// BeforeNotify adds a callback function which is run before a notice is
// reported to Honeybadger. If any function returns an error the notification
// will be skipped, otherwise it will be sent.
func (client *Client) BeforeNotify(handler func(notice *Notice) error) {
	client.beforeNotifyHandlers = append(client.beforeNotifyHandlers, handler)
}

// Notify reports the error err to the Honeybadger service.
func (client *Client) Notify(err interface{}, extra ...interface{}) (string, error) {
	extra = append([]interface{}{client.context.internal}, extra...)
	notice := newNotice(client.Config, newError(err, 2), extra...)
	for _, handler := range client.beforeNotifyHandlers {
		if err := handler(notice); err != nil {
			return "", err
		}
	}

	notifyFn := func() error {
		return client.Config.Backend.Notify(Notices, notice)
	}

	if client.Config.Sync {
		if notifyErr := notifyFn(); notifyErr != nil {
			client.Config.Logger.Printf("notify error: %v\n", notifyErr)
			return "", notifyErr
		}
	} else {
		if workerPushErr := client.worker.Push(notifyFn); workerPushErr != nil {
			client.Config.Logger.Printf("worker error: %v\n", workerPushErr)
			return "", workerPushErr
		}
	}

	return notice.Token, nil
}

// Monitor automatically reports panics which occur in the function it's called
// from. Must be deferred.
func (client *Client) Monitor() {
	if err := recover(); err != nil {
		client.Notify(newError(err, 2))
		client.Flush()
		panic(err)
	}
}

// Handler returns an http.Handler function which automatically reports panics
// to Honeybadger and then re-panics.
func (client *Client) Handler(h http.Handler) http.Handler {
	if h == nil {
		h = http.DefaultServeMux
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				client.Notify(newError(err, 2), Params(r.Form), getCGIData(r), *r.URL)
				panic(err)
			}
		}()
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (client *Client) GinRecoveryHandler() func(*gin.Context, interface{}) {
	return func(ctx *gin.Context, err interface{}) {
		r := ctx.Request
		client.Notify(newError(err, 2), Params(r.Form), getCGIData(r), *r.URL)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"err":     err,
		})
	}
}

// New returns a new instance of Client.
func New(c Configuration) *Client {
	config := newConfig(c)
	worker := newBufferedWorker(config)

	client := Client{
		Config:  config,
		worker:  worker,
		context: newContextSync(),
	}

	return &client
}

func getCGIData(request *http.Request) CGIData {
	cgiData := CGIData{}
	replacer := strings.NewReplacer("-", "_")
	for k, v := range request.Header {
		key := "HTTP_" + replacer.Replace(strings.ToUpper(k))
		cgiData[key] = v[0]
	}
	return cgiData
}

func GetCGIData(r *http.Request) CGIData {
	return getCGIData(r)
}

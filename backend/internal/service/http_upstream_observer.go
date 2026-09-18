package service

import (
	"net/http"
)

// HTTPUpstreamObserver can wrap a response body without changing upstream behavior.
// Implementations must be non-blocking on the request path and must not consume the body.
type HTTPUpstreamObserver interface {
	ObserveHTTPResponse(req *http.Request, accountID int64, resp *http.Response) *http.Response
}

// HTTPUpstreamObserverSetter is implemented by the concrete shared HTTP transport.
type HTTPUpstreamObserverSetter interface {
	SetHTTPUpstreamObserver(observer HTTPUpstreamObserver)
}

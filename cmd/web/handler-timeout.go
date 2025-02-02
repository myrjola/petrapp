package main

import (
	"net/http"
	"time"
)

const timeoutBody = `<html lang="en">
<head><title>Timeout</title></head>
<body>
<h1>Timeout</h1>
<div>
    <button type="button">
        <span>Retry</span>
        <script>
          document.currentScript.parentElement.addEventListener('click', function () {
            location.reload();
          });
        </script>
    </button>
</div>
</body>
</html>
`

// timeout responds with a 503 Service Unavailable error when the handler does not meet the deadline.
func timeout(h http.Handler) http.Handler {
	// We want the timeout to be a little shorter than the server's read timeout so that the
	// timeout handler has a chance to respond before the server closes the connection.
	defaultTimeout := time.Second
	httpHandlerTimeout := defaultTimeout - 200*time.Millisecond //nolint:mnd // 200ms
	return http.TimeoutHandler(h, httpHandlerTimeout, timeoutBody)
}

package main

import (
	"fmt"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/models"
	"log/slog"
	"net/http"
	"time"
)

type investigateTargetTemplateData struct {
	BaseTemplateData

	Investigation models.Investigation
}

func (app *application) investigateTargetGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	investigationTargetID := r.PathValue("investigationTargetID")
	investigation, err := app.investigations.Get(ctx, investigationTargetID, userID)
	if err != nil {
		app.serverError(w, r, errors.Wrap(
			err,
			"get investigation",
			slog.String("investigation_target_id", investigationTargetID),
		))
		return
	}
	data := investigateTargetTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Investigation:    *investigation,
	}
	app.render(w, r, http.StatusOK, "investigatetarget", data)
}

func (app *application) investigateTargetPOST(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	investigationTargetID := r.PathValue("investigationTargetID")
	investigation, err := app.investigations.Get(ctx, investigationTargetID, userID)
	if err != nil {
		app.serverError(w, r, errors.Wrap(
			err,
			"get investigation",
			slog.String("investigation_target_id", investigationTargetID),
		))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		app.logger.LogAttrs(ctx, slog.LevelWarn, "expected http.ResponseWriter to be an http.Flusher")
	}

	// Respond with chunked transfer encoding to stream the response.
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Writing chunks to the response
	for i := range 100 {
		// Simulate some processing
		time.Sleep(10 * time.Millisecond) //nolint:mnd // 10 milliseconds

		// Write a chunk to the response
		_, err = fmt.Fprintf(w, "Chunk %d, %s\n", i, investigation.Target.Name)
		if err != nil {
			return
		}
		// Flush the buffer to ensure the chunk is sent immediately
		app.logger.LogAttrs(ctx, slog.LevelDebug, "flushing chunk")
		flusher.Flush()
	}

	// http.Redirect(w, r, r.RequestURI, http.StatusSeeOther)
}

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

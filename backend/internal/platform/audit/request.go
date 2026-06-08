package audit

import (
	"context"
	"net/http"
)

type RequestRecorder interface {
	Record(ctx context.Context, entry Entry) error
}

func RecordRequest(recorder RequestRecorder, r *http.Request, entry Entry) {
	if recorder == nil || r == nil {
		return
	}
	entry.IP = RequestIP(r)
	entry.UserAgent = r.UserAgent()
	_ = recorder.Record(r.Context(), entry)
}

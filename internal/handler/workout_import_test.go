package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type importHandlerService struct {
	file    []byte
	opts    model.ImportOptions
	key     string
	outcome model.ImportCommitOutcome
	err     error
	deleted uuid.UUID
}

func (s *importHandlerService) Preview(_ context.Context, _ uuid.UUID, data []byte, opts model.ImportOptions) (*model.ImportPreview, error) {
	s.file, s.opts = data, opts
	if s.err != nil {
		return nil, s.err
	}
	return &model.ImportPreview{Source: model.ImportSourceStrong, UnmatchedExercises: []model.UnmatchedExercise{}}, nil
}

func (s *importHandlerService) Commit(_ context.Context, _ uuid.UUID, data []byte, opts model.ImportOptions, key string) (*model.WorkoutImport, model.ImportCommitOutcome, error) {
	s.file, s.opts, s.key = data, opts, key
	if s.err != nil {
		return nil, s.outcome, s.err
	}
	return &model.WorkoutImport{ID: uuid.New(), Source: model.ImportSourceHevy}, s.outcome, nil
}

func (s *importHandlerService) List(context.Context, uuid.UUID) ([]model.WorkoutImport, error) {
	return []model.WorkoutImport{}, s.err
}

func (s *importHandlerService) Delete(_ context.Context, _, id uuid.UUID) error {
	s.deleted = id
	return s.err
}

type importPart struct {
	name, filename, value string
}

func importRequest(t *testing.T, idempotencyKey string, parts ...importPart) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		var w interface{ Write([]byte) (int, error) }
		var err error
		if part.filename != "" {
			w, err = writer.CreateFormFile(part.name, part.filename)
		} else {
			w, err = writer.CreateFormField(part.name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(part.value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := newReq(t, http.MethodPost, "/", body.String(), uuid.New())
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req
}

func TestWorkoutImportHandlerPreviewReadsForm(t *testing.T) {
	svc := &importHandlerService{}
	rec := httptest.NewRecorder()
	NewWorkoutImportHandler(svc).Preview(rec, importRequest(t, "",
		importPart{name: "source_weight_unit", value: "lb"},
		importPart{name: "file", filename: "strong.csv", value: "a,b\n"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if string(svc.file) != "a,b\n" || svc.opts.SourceWeightUnit != "lb" {
		t.Fatalf("form = %q %+v", svc.file, svc.opts)
	}
}

func TestWorkoutImportHandlerCommitStatus(t *testing.T) {
	for _, test := range []struct {
		outcome model.ImportCommitOutcome
		status  int
	}{{model.ImportCreated, http.StatusCreated}, {model.ImportReplayed, http.StatusOK}} {
		svc := &importHandlerService{outcome: test.outcome}
		rec := httptest.NewRecorder()
		NewWorkoutImportHandler(svc).Commit(rec, importRequest(t, "op-1",
			importPart{name: "unmatched", value: "skip"},
			importPart{name: "operation_key", value: "op-1"},
			importPart{name: "file", filename: "hevy.csv", value: "x"}))
		if rec.Code != test.status || svc.key != "op-1" || svc.opts.Unmatched != "skip" {
			t.Fatalf("status=%d key=%s opts=%+v body=%s", rec.Code, svc.key, svc.opts, rec.Body.String())
		}
	}
}

func TestWorkoutImportHandlerErrors(t *testing.T) {
	file := importPart{name: "file", filename: "a.csv", value: "x"}
	cases := []struct {
		name   string
		req    func(t *testing.T) *http.Request
		commit bool
		err    error
		status int
		code   string
	}{
		{"no file", func(t *testing.T) *http.Request {
			return importRequest(t, "", importPart{name: "unmatched", value: "keep"})
		}, false, nil, 400, "BAD_REQUEST"},
		{"two files", func(t *testing.T) *http.Request { return importRequest(t, "", file, file) }, false, nil, 400, "BAD_REQUEST"},
		{"wrong part", func(t *testing.T) *http.Request {
			return importRequest(t, "", importPart{name: "upload", filename: "a.csv", value: "x"})
		}, false, nil, 400, "BAD_REQUEST"},
		{"not multipart", func(t *testing.T) *http.Request { return newReq(t, http.MethodPost, "/", "{}", uuid.New()) }, false, nil, 400, "BAD_REQUEST"},
		{"too large", func(t *testing.T) *http.Request {
			return importRequest(t, "", importPart{name: "file", filename: "a.csv", value: strings.Repeat("x", service.MaxImportFileBytes+1)})
		}, false, nil, 413, "REQUEST_TOO_LARGE"},
		{"long field", func(t *testing.T) *http.Request {
			return importRequest(t, "", importPart{name: "unmatched", value: strings.Repeat("k", 300)}, file)
		}, false, nil, 400, "BAD_REQUEST"},
		{"key mismatch", func(t *testing.T) *http.Request {
			return importRequest(t, "header", importPart{name: "operation_key", value: "body"}, file)
		}, true, nil, 422, "VALIDATION_ERROR"},
		{"file error", func(t *testing.T) *http.Request { return importRequest(t, "", file) }, false,
			&model.ImportFileError{Code: model.ImportCodeInvalidRow, Message: "line 3: bad", Details: map[string]any{"line": 3}}, 422, model.ImportCodeInvalidRow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewWorkoutImportHandler(&importHandlerService{err: tc.err})
			rec := httptest.NewRecorder()
			if tc.commit {
				h.Commit(rec, tc.req(t))
			} else {
				h.Preview(rec, tc.req(t))
			}
			var body struct {
				Code    string         `json:"code"`
				Details map[string]any `json:"details"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != tc.status || body.Code != tc.code {
				t.Fatalf("status=%d code=%s body=%s", rec.Code, body.Code, rec.Body.String())
			}
			if tc.err != nil && body.Details["line"] != float64(3) {
				t.Fatalf("details = %v", body.Details)
			}
		})
	}
}

func TestWorkoutImportHandlerListAndDelete(t *testing.T) {
	svc := &importHandlerService{}
	rec := httptest.NewRecorder()
	NewWorkoutImportHandler(svc).List(rec, newReq(t, http.MethodGet, "/", nil, uuid.New()))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"imports":[]}` {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	id := uuid.New()
	rec = httptest.NewRecorder()
	NewWorkoutImportHandler(svc).Delete(rec, guidedHandlerRequest(t, http.MethodDelete, "/", nil, uuid.New(), "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent || svc.deleted != id {
		t.Fatalf("delete status=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	NewWorkoutImportHandler(&importHandlerService{err: &model.NotFoundError{Message: "import not found"}}).Delete(rec,
		guidedHandlerRequest(t, http.MethodDelete, "/", nil, uuid.New(), "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing delete status=%d", rec.Code)
	}
}

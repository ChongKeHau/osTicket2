package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
)

func init() { gin.SetMode(gin.TestMode) }

type envelope struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func do(t *testing.T, r *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var e envelope
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	return w, e
}

func TestFailMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{apperr.Validation("subject", "required"), 400, "validation_failed"},
		{apperr.ErrUnauthorized, 401, "unauthorized"},
		{apperr.ErrForbidden, 403, "forbidden"},
		{errors.Join(errors.New("wrapped"), apperr.ErrNotFound), 404, "not_found"},
		{apperr.ErrConflict, 409, "conflict"},
		{apperr.ErrPayloadTooLarge, 413, "payload_too_large"},
		{errors.New("boom"), 500, "internal"},
	}
	for _, tc := range cases {
		r := gin.New()
		r.GET("/x", func(c *gin.Context) { Fail(c, tc.err) })
		w, e := do(t, r, http.MethodGet, "/x", "")
		if w.Code != tc.status || e.Error.Code != tc.code {
			t.Errorf("%v: got %d %q want %d %q", tc.err, w.Code, e.Error.Code, tc.status, tc.code)
		}
		if tc.status == 500 && e.Error.Message != "internal server error" {
			t.Errorf("internal error leaked message %q", e.Error.Message)
		}
		if tc.status == 400 && e.Error.Fields["subject"] != "required" {
			t.Errorf("fields missing: %+v", e.Error.Fields)
		}
	}
}

func TestBindJSONReportsJSONFieldNames(t *testing.T) {
	type in struct {
		Subject string `json:"subject" binding:"required,max=5"`
		Email   string `json:"requester_email" binding:"required,email"`
	}
	r := gin.New()
	r.POST("/x", func(c *gin.Context) {
		var v in
		if !BindJSON(c, &v) {
			return
		}
		c.Status(204)
	})
	w, e := do(t, r, http.MethodPost, "/x", `{"subject":"toolong","requester_email":"nope"}`)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
	if e.Error.Fields["subject"] != "max" || e.Error.Fields["requester_email"] != "email" {
		t.Fatalf("fields: %+v", e.Error.Fields)
	}
	w, e = do(t, r, http.MethodPost, "/x", `{not json`)
	if w.Code != 400 || e.Error.Fields["body"] == "" {
		t.Fatalf("malformed json: %d %+v", w.Code, e.Error.Fields)
	}
	w, _ = do(t, r, http.MethodPost, "/x", `{"subject":"ok","requester_email":"a@b.test"}`)
	if w.Code != 204 {
		t.Fatalf("valid body status %d", w.Code)
	}
}

func TestParsePage(t *testing.T) {
	cases := []struct {
		query   string
		page    int
		size    int
		wantErr bool
	}{
		{"", 1, 25, false},
		{"page=3&page_size=10", 3, 10, false},
		{"page_size=500", 1, 100, true},
		{"page_size=0", 0, 0, true},
		{"page_size=-5", 0, 0, true},
		{"page_size=abc", 0, 0, true},
		{"page=0", 0, 0, true},
		{"page=2000000000", 0, 0, true},
		{"page=1000001", 0, 0, true},
		{"page=1000000", 1000000, 25, false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x?"+tc.query, nil)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		p, err := ParsePage(c)
		if tc.wantErr {
			var ve *apperr.ValidationError
			if !errors.As(err, &ve) {
				t.Errorf("%q: expected validation error, got %v", tc.query, err)
			}
			continue
		}
		if err != nil || p.Page != tc.page || p.PageSize != tc.size {
			t.Errorf("%q: got %+v %v", tc.query, p, err)
		}
	}
	p := Page{Page: 3, PageSize: 10}
	if p.Offset() != 20 || p.Limit() != 10 {
		t.Fatalf("offset/limit: %d %d", p.Offset(), p.Limit())
	}
}

func TestParseID(t *testing.T) {
	r := gin.New()
	r.GET("/x/:id", func(c *gin.Context) {
		id, err := ParseID(c, "id")
		if err != nil {
			Fail(c, err)
			return
		}
		c.JSON(200, gin.H{"id": id})
	})
	w, _ := do(t, r, http.MethodGet, "/x/42", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "42") {
		t.Fatalf("numeric id: %d %s", w.Code, w.Body.String())
	}
	w, e := do(t, r, http.MethodGet, "/x/abc", "")
	if w.Code != 400 || e.Error.Fields["id"] == "" {
		t.Fatalf("non-numeric id: %d %+v", w.Code, e.Error)
	}
}

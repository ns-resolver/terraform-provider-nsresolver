package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenAndDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("NSToken") != "tok" {
			w.WriteHeader(401)
			w.Write([]byte(`{"status":"failed","reason":"invalid token"}`))
			return
		}
		if r.URL.Path != "/v1/example.com/42" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"status":"ok","record":{"id":42,"type":"MX","record":"@","fqdn":"example.com","answer":"mx.example.net","ttl":300,"priority":10}}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/v1/", "tok", "test")
	rec, err := c.GetRecord(context.Background(), "example.com", 42)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != 42 || rec.Answer != "mx.example.net" || rec.Priority == nil || *rec.Priority != 10 || rec.Weight != nil {
		t.Errorf("decoded %+v", rec)
	}

	c.Token = "bad"
	_, err = c.GetRecord(context.Background(), "example.com", 42)
	var e *Error
	if err == nil || !asErr(err, &e) || e.StatusCode != 401 || e.Reason != "invalid token" {
		t.Errorf("want 401 invalid token, got %v", err)
	}
}

func TestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"status":"failed","reason":"record not found","detail":"No record with that id in this zone."}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "t", "").GetRecord(context.Background(), "example.com", 1)
	if !IsNotFound(err) {
		t.Errorf("want not found, got %v", err)
	}
}

func TestRetriesOn429WithRetryAfter(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			w.Write([]byte(`{"status":"failed","reason":"rate limit"}`))
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["answer"] != "192.0.2.1" {
			t.Errorf("body not resent: %v", body)
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"status":"accepted","record":{"id":7,"type":"A","record":"www","answer":"192.0.2.1","ttl":3600}}`))
	}))
	defer srv.Close()
	answer, name := "192.0.2.1", "www"
	rec, err := New(srv.URL, "t", "").CreateRecord(context.Background(), "example.com", RecordInput{Type: "A", Record: &name, Answer: &answer})
	if err != nil || rec.ID != 7 || n != 3 {
		t.Errorf("rec=%v err=%v attempts=%d", rec, err, n)
	}
}

func TestPostNotRetriedOn500(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := New(srv.URL, "t", "")
	c.MaxWait = time.Millisecond
	if _, err := c.CreateZone(context.Background(), "example.com"); err == nil || n != 1 {
		t.Errorf("err=%v attempts=%d", err, n)
	}
	n = 0
	if _, _, err := c.GetZone(context.Background(), "example.com"); err == nil || n != int32(c.MaxRetries+1) {
		t.Errorf("GET: err=%v attempts=%d", err, n)
	}
}

func TestSettingsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Type     string         `json:"type"`
			Settings map[string]any `json:"settings"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Type != "ZONE" || len(body.Settings) != 2 || body.Settings["refresh"] != float64(1800) || body.Settings["retry"] != float64(900) {
			t.Errorf("body %+v", body)
		}
		w.Write([]byte(`{"status":"accepted","type":"zone","settings":{"default-ttl":3600,"refresh":1800,"retry":900,"expire":1209600,"negative-answer":300}}`))
	}))
	defer srv.Close()
	r, rt := int64(1800), int64(900)
	s, err := New(srv.URL, "t", "").SetSettings(context.Background(), "example.com", Settings{Refresh: &r, Retry: &rt})
	if err != nil || *s.Expire != 1209600 {
		t.Errorf("s=%+v err=%v", s, err)
	}
}

func asErr(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

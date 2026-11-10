// Package client is a small client for the NSResolver HTTPS API (https://api.nsresolver.com/v1).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultEndpoint is the base URL of version 1 of the API.
const DefaultEndpoint = "https://api.nsresolver.com/v1"

// Client talks to the API with one account's token.
type Client struct {
	Endpoint   string
	Token      string
	UserAgent  string
	HTTPClient *http.Client
	// MaxRetries is how many times a rate-limited (429) request, or an idempotent request that
	// failed with a network error or a 5xx response, is retried.
	MaxRetries int
	// MaxWait caps the wait before a retry.
	MaxWait time.Duration
}

// New returns a client with sensible defaults.
func New(endpoint, token, userAgent string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		Endpoint:   strings.TrimRight(endpoint, "/"),
		Token:      token,
		UserAgent:  userAgent,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		MaxRetries: 5,
		MaxWait:    60 * time.Second,
	}
}

// Error is a refused request: {"status": "failed", "reason": ..., "detail": ...}.
type Error struct {
	StatusCode int
	Reason     string
	Detail     string
	Errors     map[string]string
}

func (e *Error) Error() string {
	msg := e.Reason
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return fmt.Sprintf("NSResolver API error (HTTP %d): %s", e.StatusCode, msg)
}

// IsNotFound reports whether err is a 404 from the API (zone, record or lookup not found).
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// Settings are a zone's settings, in seconds. Pointers so a partial change can be sent.
type Settings struct {
	DefaultTTL     *int64 `json:"default-ttl,omitempty"`
	Refresh        *int64 `json:"refresh,omitempty"`
	Retry          *int64 `json:"retry,omitempty"`
	Expire         *int64 `json:"expire,omitempty"`
	NegativeAnswer *int64 `json:"negative-answer,omitempty"`
}

// Zone as returned by the API.
type Zone struct {
	Zone      string   `json:"zone"`
	ID        int64    `json:"id"`
	Status    string   `json:"status"`
	Published bool     `json:"published"`
	Serial    int64    `json:"serial"`
	Records   int64    `json:"records"`
	Settings  Settings `json:"settings"`
}

// Record as returned by the API. Priority, Weight, Port, Flags and Tag are set only for the
// types that use them (MX/SRV, SRV, CAA).
type Record struct {
	ID       int64   `json:"id"`
	Type     string  `json:"type"`
	Record   string  `json:"record"`
	FQDN     string  `json:"fqdn"`
	Answer   string  `json:"answer"`
	TTL      int64   `json:"ttl"`
	Priority *int64  `json:"priority,omitempty"`
	Weight   *int64  `json:"weight,omitempty"`
	Port     *int64  `json:"port,omitempty"`
	Flags    *int64  `json:"flags,omitempty"`
	Tag      *string `json:"tag,omitempty"`
}

// RecordInput is a record to create, or the fields to change (nil = unchanged / not sent).
type RecordInput struct {
	Type     string  `json:"type,omitempty"`
	Record   *string `json:"record,omitempty"`
	Answer   *string `json:"answer,omitempty"`
	TTL      *int64  `json:"ttl,omitempty"`
	Priority *int64  `json:"priority,omitempty"`
	Weight   *int64  `json:"weight,omitempty"`
	Port     *int64  `json:"port,omitempty"`
	Flags    *int64  `json:"flags,omitempty"`
	Tag      *string `json:"tag,omitempty"`
}

// ---- zones ----

func (c *Client) ListZones(ctx context.Context) ([]Zone, error) {
	var out struct {
		Zones []Zone `json:"zones"`
	}
	err := c.do(ctx, http.MethodGet, "/zones", nil, &out)
	return out.Zones, err
}

func (c *Client) CreateZone(ctx context.Context, name string) (*Zone, error) {
	var out struct {
		Zone Zone `json:"zone"`
	}
	if err := c.do(ctx, http.MethodPost, "/zones", map[string]string{"zone": name}, &out); err != nil {
		return nil, err
	}
	return &out.Zone, nil
}

// GetZone returns the zone and all its records.
func (c *Client) GetZone(ctx context.Context, name string) (*Zone, []Record, error) {
	var out struct {
		Zone    Zone     `json:"zone"`
		Records []Record `json:"records"`
	}
	if err := c.do(ctx, http.MethodGet, "/"+url.PathEscape(name)+"/", nil, &out); err != nil {
		return nil, nil, err
	}
	return &out.Zone, out.Records, nil
}

func (c *Client) DeleteZone(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/"+url.PathEscape(name)+"/", nil, nil)
}

// SetSettings checks and saves the given settings together; returns all five.
func (c *Client) SetSettings(ctx context.Context, zone string, s Settings) (*Settings, error) {
	var out struct {
		Settings Settings `json:"settings"`
	}
	body := map[string]any{"type": "ZONE", "settings": s}
	if err := c.do(ctx, http.MethodPost, "/"+url.PathEscape(zone)+"/", body, &out); err != nil {
		return nil, err
	}
	return &out.Settings, nil
}

// ---- records ----

func (c *Client) CreateRecord(ctx context.Context, zone string, in RecordInput) (*Record, error) {
	var out struct {
		Record Record `json:"record"`
	}
	if err := c.do(ctx, http.MethodPost, "/"+url.PathEscape(zone)+"/", in, &out); err != nil {
		return nil, err
	}
	return &out.Record, nil
}

func (c *Client) GetRecord(ctx context.Context, zone string, id int64) (*Record, error) {
	var out struct {
		Record Record `json:"record"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/%s/%d", url.PathEscape(zone), id), nil, &out); err != nil {
		return nil, err
	}
	return &out.Record, nil
}

// UpdateRecord changes the fields set in `in`; the others keep their values. The type can't change.
func (c *Client) UpdateRecord(ctx context.Context, zone string, id int64, in RecordInput) (*Record, error) {
	var out struct {
		Record Record `json:"record"`
	}
	if err := c.do(ctx, http.MethodPut, fmt.Sprintf("/%s/%d", url.PathEscape(zone), id), in, &out); err != nil {
		return nil, err
	}
	return &out.Record, nil
}

func (c *Client) DeleteRecord(ctx context.Context, zone string, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/%s/%d", url.PathEscape(zone), id), nil, nil)
}

// ---- transport ----

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	idempotent := method != http.MethodPost
	for attempt := 0; ; attempt++ {
		wait, err := c.once(ctx, method, path, payload, out)
		if err == nil {
			return nil
		}
		retry := false
		var apiErr *Error
		switch {
		case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests:
			retry = true
		case errors.As(err, &apiErr) && apiErr.StatusCode >= 500 && idempotent:
			retry = true
		case !errors.As(err, &apiErr) && idempotent && ctx.Err() == nil:
			retry = true // network error
		}
		if !retry || attempt >= c.MaxRetries {
			return err
		}
		if wait <= 0 {
			wait = time.Duration(1<<attempt) * time.Second
		}
		if wait > c.MaxWait {
			wait = c.MaxWait
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// once sends one request. On a 429 it returns the Retry-After wait.
func (c *Client) once(ctx context.Context, method, path string, payload []byte, out any) (time.Duration, error) {
	var rdr io.Reader
	if payload != nil {
		rdr = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Endpoint+path, rdr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("NSToken", c.Token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return 0, err
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if out == nil || len(data) == 0 {
			return 0, nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return 0, fmt.Errorf("NSResolver API: unexpected response: %w", err)
		}
		return 0, nil
	}
	apiErr := &Error{StatusCode: res.StatusCode}
	var fail struct {
		Reason string            `json:"reason"`
		Detail string            `json:"detail"`
		Errors map[string]string `json:"errors"`
	}
	if json.Unmarshal(data, &fail) == nil {
		apiErr.Reason, apiErr.Detail, apiErr.Errors = fail.Reason, fail.Detail, fail.Errors
	}
	var wait time.Duration
	if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s >= 0 {
		wait = time.Duration(s) * time.Second
	}
	return wait, apiErr
}

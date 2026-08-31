// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// NewClient is the exported seam every out-of-tree caller goes through —
// cli, sdk and netrunner all hand it a bare node URI and let it build the
// path. luxd serves exactly one prefix, /v1 (node/server/http/server.go),
// so the prefix this function bakes in decides whether those callers reach
// the node at all.
func TestNewClientRequestsV1Paths(t *testing.T) {
	seen := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "C")
	if err := c.StartCPUProfiler(context.Background()); err != nil {
		t.Fatalf("StartCPUProfiler: %v", err)
	}

	got := <-seen
	if want := "/v1/bc/C/admin"; got != want {
		t.Fatalf("admin endpoint = %q, want %q", got, want)
	}
	if strings.Contains(got, "/ext/") {
		t.Fatalf("request still carries the legacy /ext prefix: %q", got)
	}

	if _, err := c.GetCurrentValidators(context.Background(), nil); err != nil {
		t.Fatalf("GetCurrentValidators: %v", err)
	}
	if got, want := <-seen, "/v1/bc/C/validators"; got != want {
		t.Fatalf("validators endpoint = %q, want %q", got, want)
	}
}

// The chain segment must be interpolated, not hardcoded, so a blockchain ID
// works as well as an alias.
func TestNewClientUsesChainArgument(t *testing.T) {
	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	const chainID = "2T11SNJM9KYqSenoPb5yv6qahmj8HLefCjxt4gTD2iQWTHwdRu"
	if err := NewClient(srv.URL, chainID).StartCPUProfiler(context.Background()); err != nil {
		t.Fatalf("StartCPUProfiler: %v", err)
	}
	if got, want := <-seen, "/v1/bc/"+chainID+"/admin"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

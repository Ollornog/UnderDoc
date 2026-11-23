// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// testVaultKey is a key for tests only.
const testVaultKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func testVault(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.New(testVaultKey)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func inMain() context.Context { return tenant.With(context.Background(), tenant.Main) }

// dbSite is a site with a database of its own and a key for the vault.
func dbSite(t *testing.T, options Options) (*site, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	options.DB = database
	if options.Vault == nil {
		options.Vault = testVault(t)
	}
	return newSite(t, options, sample), database
}

// client is a browser behind the gate for tests: it keeps cookies, sends the host and the gate's headers, and says that
// its requests come from the same site.
type client struct {
	t         *testing.T
	s         *site
	host      string
	cookies   map[string]string
	crossSite bool
	headers   map[string]string
}

func newClient(t *testing.T, s *site, host string) *client {
	return &client{t: t, s: s, host: host, cookies: map[string]string{}, headers: map[string]string{}}
}

func (c *client) do(method, target, contentType, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = c.host
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if c.crossSite {
		request.Header.Set("Sec-Fetch-Site", "cross-site")
	} else {
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for name, value := range c.headers {
		request.Header.Set(name, value)
	}
	for name, value := range c.cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	recorder := httptest.NewRecorder()
	c.s.http.ServeHTTP(recorder, request)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
		} else {
			c.cookies[cookie.Name] = cookie.Value
		}
	}
	return recorder
}

func (c *client) get(target string) *httptest.ResponseRecorder {
	return c.do(http.MethodGet, target, "", "")
}

func (c *client) form(target string, values url.Values) *httptest.ResponseRecorder {
	return c.do(http.MethodPost, target, "application/x-www-form-urlencoded", values.Encode())
}

func (c *client) post(target, body string) *httptest.ResponseRecorder {
	return c.do(http.MethodPost, target, "application/json", body)
}

// between returns what stands in s between start and end, or "".
func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return ""
	}
	return s[:j]
}

func newClientWith(t *testing.T, s *site, headers map[string]string) *client {
	c := newClient(t, s, "wiki.example")
	c.headers = headers
	return c
}

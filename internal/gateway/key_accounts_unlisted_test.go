package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/provider"
)

// Two ChatGPT accounts on, the second's own model list the Free plan's
// (the one it was signed in with before it was upgraded) while the
// first's has every model: a gateway key held to the second account
// alone was refused "every account outside the key's accounts" for a
// model only the first lists, though with the second the only account
// on the same request was sent to it. The accounts the key may not use
// are not who "none lists it" is judged over: the key's second account is
// asked, as it is with no other account to list the model.
func TestKeyHeldToAccountWhoseListLacksModelStillServed(t *testing.T) {
	codexSignedIn(t, "spare@example.com")
	var mu sync.Mutex
	var tried []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct := r.Header.Get("chatgpt-account-id")
		if strings.HasSuffix(r.URL.Path, "/models") {
			io.WriteString(w, codexPlanList(acct == "acct-2"))
			return
		}
		mu.Lock()
		tried = append(tried, acct)
		mu.Unlock()
		io.WriteString(w, sse(
			`data: {"type":"response.created","response":{"id":"r1","model":"gpt-6-astra"}}`,
			`data: {"type":"response.output_text.delta","delta":"pong"}`,
			`data: {"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":7,"output_tokens":1}}}`))
	}))
	t.Cleanup(up.Close)
	was := provider.CodexBase
	provider.CodexBase = up.URL + "/backend-api/codex"
	t.Cleanup(func() { provider.CodexBase = was })

	p, err := provider.Find("codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	spare := &provider.Account{Agent: "codex", User: "spare@example.com"}
	if spare.Lists("gpt-6-astra") {
		t.Fatal("the second account's list isn't the Free plan's")
	}

	keys, secrets := newCaller(t, "Both", "First", "Second")
	for i, as := range [][]string{nil, {"codex/me@example.com"}, {"codex/spare@example.com"}} {
		if _, err := access.Update("accounts-key", access.Change{Key: keys[i].ID, Accounts: as}); err != nil {
			t.Fatal(err)
		}
	}
	s := New()
	ask := func(secret string) (int, string) {
		t.Helper()
		mu.Lock()
		tried = nil
		mu.Unlock()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"codex/gpt-6-astra","stream":true,"input":"ping"}`))
		req.Header.Set("Authorization", "Bearer "+secret)
		s.Handler().ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	for i, c := range []struct{ who, acct string }{
		{"no accounts held", "acct-1"}, // the account that lists it, the second kept out as before
		{"held to the first", "acct-1"},
		{"held to the second", "acct-2"},
	} {
		code, body := ask(secrets[i])
		mu.Lock()
		got := strings.Join(tried, ",")
		mu.Unlock()
		if code != 200 || got != c.acct {
			t.Errorf("%s: status %d, asked %q, want %q: %s", c.who, code, got, c.acct, body)
		}
	}
}

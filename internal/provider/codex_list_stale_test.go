package provider

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
)

func codexSignInPlan(t *testing.T, home, email, plan string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"id_token":      fakeJWT(map[string]any{"email": email, "https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": plan}}),
			"access_token":  fakeJWT(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())}),
			"refresh_token": "r-" + plan, "account_id": "acct-" + email,
		},
	})
}

// An account's own model list is the plan it was fetched for: signed in
// anew at another plan (a Free account upgraded to Plus), or forgotten and
// added again, it is taken to have every model until the next fetch, not
// held to the old plan's.
func TestCodexAccountListDroppedWithItsPlan(t *testing.T) {
	home := signIn(t)
	rememberLogins(true)
	acct := &Account{Agent: "codex", User: "up@example.com"}
	free := func() {
		t.Helper()
		if err := catalog.SaveLive(accountModels("codex", acct.User), "", []catalog.Model{{ID: "gpt-6-luna"}}); err != nil {
			t.Fatal(err)
		}
		if acct.Lists("gpt-6-astra") {
			t.Fatal("the Free list has a Plus model")
		}
	}

	codexSignInPlan(t, home, acct.User, "free")
	rememberLogins(true)
	free()
	codexSignInPlan(t, home, acct.User, "free")
	rememberLogins(true)
	if acct.Lists("gpt-6-astra") {
		t.Fatal("a sign-in at the same plan dropped the list")
	}
	codexSignInPlan(t, home, acct.User, "plus")
	rememberLogins(true)
	if !acct.Lists("gpt-6-astra") {
		t.Fatal("the Free plan's list outlived the upgrade")
	}

	free()
	if err := ForgetLogin("codex", acct.User); err != nil {
		t.Fatal(err)
	}
	if !acct.Lists("gpt-6-astra") {
		t.Fatal("a forgotten account kept its list")
	}
}

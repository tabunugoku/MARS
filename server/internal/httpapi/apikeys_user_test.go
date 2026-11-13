package httpapi

import "testing"

// A regular user manages their own API keys: create, list, use as a Bearer
// token, delete.
func TestUserApiKeys(t *testing.T) {
	e := newEnv(t)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user"}, 201, nil)
	r := e.login("読者", "reader pass")

	var created ApiKeyCreated
	r.json("POST", "/api/v1/me/api-keys", map[string]any{"name": "Mihon"}, 201, &created)

	var keys []ApiKey
	r.json("GET", "/api/v1/me/api-keys", nil, 200, &keys)
	if len(keys) != 1 || keys[0].Id != created.Id || keys[0].Name != "Mihon" {
		t.Fatalf("keys after create: %+v", keys)
	}

	bot := client{t: t, h: e.h, bearer: created.Key}
	bot.json("GET", "/api/v1/me", nil, 200, nil)
	bot.json("GET", "/api/v1/libraries", nil, 200, nil)

	r.json("DELETE", "/api/v1/me/api-keys/"+created.Id, nil, 204, nil)
	keys = nil
	r.json("GET", "/api/v1/me/api-keys", nil, 200, &keys)
	if len(keys) != 0 {
		t.Fatalf("keys after delete: %+v", keys)
	}
	bot.json("GET", "/api/v1/me", nil, 401, nil)
}

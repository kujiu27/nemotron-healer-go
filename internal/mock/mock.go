package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Server provides an in-process, labeled mock of Nebius Token Factory and Tavily APIs.
type Server struct {
	*httptest.Server
}

// Benchmark mock data definitions
const (
	// AHB-01: FastAPI Async Deadlock / Concurrency Race
	ahb01Patch = "```diff\n--- a/account.py\n+++ b/account.py\n@@ -1,21 +1,25 @@\n \"\"\"Concurrent banking transaction service suffering from an async race condition.\"\"\"\n \n import asyncio\n+import threading\n from typing import Dict\n \n \n class BankLedger:\n     def __init__(self, initial_balance: float = 1000.0):\n         self.balance = initial_balance\n+        self._lock = threading.Lock()\n \n     async def transfer(self, amount: float):\n         \"\"\"Simulate async transfer with non-atomic state mutation (Race Condition).\"\"\"\n-        current = self.balance\n-        # Simulate I/O latency exposing race condition\n-        await asyncio.sleep(0.001)\n-        self.balance = current - amount\n+        with self._lock:\n+            current = self.balance\n+            # Simulate I/O latency exposing race condition\n+            await asyncio.sleep(0.001)\n+            self.balance = current - amount\n \n     async def deposit(self, amount: float):\n-        current = self.balance\n-        await asyncio.sleep(0.001)\n-        self.balance = current + amount\n+        with self._lock:\n+            current = self.balance\n+            await asyncio.sleep(0.001)\n+            self.balance = current + amount\n```\n[TARGET_FILE]account.py[/TARGET_FILE]"
	ahb01Triage = "ERROR_LINE: assert ledger.balance == 0.0\nCAUSE: concurrent transfer and deposit coroutines mutate balance without atomic synchronization\nQUERY: python asyncio shared balance synchronization lock"
	ahb01Adv    = "```python\nimport asyncio\nfrom account import BankLedger\n\ndef test_adversarial_balance_invariants():\n    ledger = BankLedger(100.0)\n    async def _run():\n        await asyncio.gather(ledger.transfer(50.0), ledger.deposit(50.0))\n        assert ledger.balance == 100.0\n    asyncio.run(_run())\n```"

	// AHB-02: Cascading Deadlock Trap
	ahb02Patch = "```diff\n--- a/service.py\n+++ b/service.py\n@@ -10,7 +10,8 @@\n         self.engine = LedgerEngine(initial_balance=initial_balance)\n \n     async def single_transfer(self, account_id: str, amount: float):\n-        return await self.engine.mutate_balance(account_id, -amount)\n+        async with self.engine.lock:\n+            return await self.engine.mutate_balance(account_id, -amount)\n \n     async def atomic_batch_transfer(self, account_id: str, amounts: List[float]):\n```\n[TARGET_FILE]service.py[/TARGET_FILE]"
	ahb02Triage = "ERROR_LINE: assert srv.engine.balance == 1000.0\nCAUSE: single_transfer mutates balance without acquiring transaction lock\nQUERY: python asyncio shared lock state synchronization"
	ahb02Adv    = "```python\nimport asyncio\nfrom service import BankingService\n\ndef test_adversarial_concurrency_stress():\n    async def _run():\n        srv = BankingService(initial_balance=5000.0)\n        tasks = [srv.single_transfer(\"acc_stress\", 50.0) for _ in range(20)]\n        await asyncio.gather(*tasks)\n        assert srv.engine.balance == 4000.0\n    asyncio.run(_run())\n```"

	// AHB-03: Pydantic v2 Migration
	ahb03Patch = "```diff\n--- a/models.py\n+++ b/models.py\n@@ -1,7 +1,7 @@\n \"\"\"Sample application models written with legacy Pydantic v1 syntax.\"\"\"\n \n from typing import List, Optional\n-from pydantic import BaseModel, validator\n+from pydantic import BaseModel, ConfigDict, field_validator\n \n \n class Item(BaseModel):\n@@ -10,10 +10,10 @@\n     price: float\n     tags: List[str] = []\n \n-    class Config:\n-        orm_mode = True\n+    model_config = ConfigDict(from_attributes=True)\n \n-    @validator(\"price\")\n+    @field_validator(\"price\")\n+    @classmethod\n     def check_price(cls, v):\n         if v <= 0:\n             raise ValueError(\"Price must be positive\")\n@@ -27,4 +27,7 @@\n \n     @classmethod\n     def from_raw(cls, data: dict):\n-        return cls.parse_obj(data)\n+        return cls.model_validate(data)\n+\n+    def to_dict(self) -> dict:\n+        return self.model_dump()\n```\n[TARGET_FILE]models.py[/TARGET_FILE]"
	ahb03Triage = "ERROR_LINE: @validator(\"price\")\nCAUSE: pydantic v1 validator and Config.orm_mode removed in pydantic v2\nQUERY: pydantic v2 migration validator to field_validator ConfigDict"
	ahb03Adv    = "```python\nimport pytest\nfrom models import Item\n\ndef test_adversarial_negative_price():\n    with pytest.raises(ValueError):\n        Item(name=\"invalid\", price=-10.0)\n```"

	// AHB-04: SQL Injection Remediation
	ahb04Patch = "```diff\n--- a/repo.py\n+++ b/repo.py\n@@ -17,9 +17,9 @@\n \n     def get_user_by_username(self, username: str) -> Optional[Dict]:\n         # VULNERABLE: Direct string formatting vulnerable to SQL Injection\n-        query = f\"SELECT id, username, role FROM users WHERE username = '{username}'\"\n+        query = \"SELECT id, username, role FROM users WHERE username = ?\"\n         cursor = self.conn.cursor()\n-        cursor.execute(query)\n+        cursor.execute(query, (username,))\n         row = cursor.fetchone()\n         if row:\n             return {\"id\": row[0], \"username\": row[1], \"role\": row[2]}\n```\n[TARGET_FILE]repo.py[/TARGET_FILE]"
	ahb04Triage = "ERROR_LINE: cursor.execute(query)\nCAUSE: raw f-string SQL query vulnerable to SQL injection bypass\nQUERY: sqlite3 parameterized query placeholder sql injection prevention"
	ahb04Adv    = "```python\nfrom repo import UserRepository\n\ndef test_adversarial_sql_injection_probe():\n    repo = UserRepository()\n    assert repo.get_user_by_username(\"admin'--\") is None\n```"

	// AHB-05: Go Concurrency Data Race
	ahb05Patch = "```diff\n--- a/cache.go\n+++ b/cache.go\n@@ -1,8 +1,12 @@\n package cache\n \n-import \"time\"\n+import (\n+\t\"sync\"\n+\t\"time\"\n+)\n \n type MetricCache struct {\n+\tmu   sync.RWMutex\n \tdata map[string]int\n }\n \n@@ -14,11 +18,15 @@\n \n // Record updates counter without proper synchronization, causing Go runtime data race.\n func (c *MetricCache) Record(key string, val int) {\n+\tc.mu.Lock()\n+\tdefer c.mu.Unlock()\n \tcurrent := c.data[key]\n \ttime.Sleep(100 * time.Microsecond)\n \tc.data[key] = current + val\n }\n \n func (c *MetricCache) Get(key string) int {\n+\tc.mu.RLock()\n+\tdefer c.mu.RUnlock()\n \treturn c.data[key]\n }\n```\n[TARGET_FILE]cache.go[/TARGET_FILE]"
	ahb05Triage = "ERROR_LINE: current := c.data[key]\nCAUSE: unsynchronized concurrent read and write to standard Go map causes fatal runtime data race\nQUERY: go concurrent map data race sync.RWMutex"
	ahb05Adv    = "```go\npackage cache\n\nimport (\n\t\"sync\"\n\t\"testing\"\n)\n\nfunc TestAdversarialConcurrentStress(t *testing.T) {\n\tc := NewMetricCache()\n\tvar wg sync.WaitGroup\n\tfor i := 0; i < 20; i++ {\n\t\twg.Add(1)\n\t\tgo func(v int) {\n\t\t\tdefer wg.Done()\n\t\t\tc.Record(\"key\", v)\n\t\t\t_ = c.Get(\"key\")\n\t\t}(i)\n\t}\n\twg.Wait()\n}\n```"

	// AHB-06: External go-diff lineHash bug
	ahb06Patch = "```diff\n--- a/diffmatchpatch/diff.go\n+++ b/diffmatchpatch/diff.go\n@@ -1312,18 +1312,18 @@\n func (dmp *DiffMatchPatch) diffLinesToStrings(text1, text2 string) (string, string, []string) {\n \t// '\\x00' is a valid character, but various debuggers don't like it. So we'll insert a junk entry to avoid generating a null character.\n \tlineArray := []string{\"\"} // e.g. lineArray[4] == 'Hello\\n'\n+\tlineHash := map[string]int{}\n \n \t//Each string has the index of lineArray which it points to\n-\tstrIndexArray1 := dmp.diffLinesToStringsMunge(text1, &lineArray)\n-\tstrIndexArray2 := dmp.diffLinesToStringsMunge(text2, &lineArray)\n+\tstrIndexArray1 := dmp.diffLinesToStringsMunge(text1, &lineArray, lineHash)\n+\tstrIndexArray2 := dmp.diffLinesToStringsMunge(text2, &lineArray, lineHash)\n \n \treturn intArrayToString(strIndexArray1), intArrayToString(strIndexArray2), lineArray\n }\n \n // diffLinesToStringsMunge splits a text into an array of strings, and reduces the texts to a []string.\n-func (dmp *DiffMatchPatch) diffLinesToStringsMunge(text string, lineArray *[]string) []uint32 {\n+func (dmp *DiffMatchPatch) diffLinesToStringsMunge(text string, lineArray *[]string, lineHash map[string]int) []uint32 {\n \t// Walk the text, pulling out a substring for each line. text.split('\\n') would would temporarily double our memory footprint. Modifying text would create many large strings to garbage collect.\n-\tlineHash := map[string]int{} // e.g. lineHash['Hello\\n'] == 4\n \tlineStart := 0\n \tlineEnd := -1\n \tstrs := []uint32{}\n```\n[TARGET_FILE]diffmatchpatch/diff.go[/TARGET_FILE]"
	ahb06Triage = "ERROR_LINE: diffLinesToStringsMunge(text2, &lineArray)\nCAUSE: local lineHash map not shared between text1 and text2 causes mismatched line indices\nQUERY: sergi go-diff lineHash diffLinesToStringsMunge share index"
	ahb06Adv    = "```go\npackage diffmatchpatch\n\nimport \"testing\"\n\nfunc TestAdversarialLineHashIdentical(t *testing.T) {\n\tdmp := New()\n\ta, b, lines := dmp.diffLinesToStrings(\"hello\\n\", \"hello\\n\")\n\tif a != b || len(lines) != 2 {\n\t\tt.Fatal(\"line hash sharing failure\")\n\t}\n}\n```"

	// AHB-07: External go-toml recursive-embedded struct hang
	ahb07Patch = "```diff\n--- a/unmarshaler.go\n+++ b/unmarshaler.go\n@@ -2260,11 +2260,17 @@\n \t\tbyName: map[string]structField{},\n \t\tbyFold: map[string]structField{},\n \t}\n-\taddFields(plan, t, nil)\n+\tvisited := map[reflect.Type]bool{}\n+\taddFields(plan, t, nil, visited)\n \treturn plan\n }\n \n-func addFields(plan *structPlan, t reflect.Type, prefix []int) {\n+func addFields(plan *structPlan, t reflect.Type, prefix []int, visited map[reflect.Type]bool) {\n+\tif visited[t] {\n+\t\treturn\n+\t}\n+\tvisited[t] = true\n+\tdefer func() { delete(visited, t) }()\n \tvar embedded []reflect.StructField\n \tfor i := 0; i < t.NumField(); i++ {\n \t\tf := t.Field(i)\n@@ -2344,7 +2350,7 @@\n \t\tindex = append(index, prefix...)\n \t\tidx := f.Index[0]\n \t\tindex = append(index, idx)\n-\t\taddFields(plan, ft, index)\n+\t\taddFields(plan, ft, index, visited)\n \t}\n }\n```\n[TARGET_FILE]unmarshaler.go[/TARGET_FILE]"
	ahb07Triage = "ERROR_LINE: addFields(plan, ft, index)\nCAUSE: unmarshaling plan enters infinite recursive loop on mutually or self-embedded structs without cycle detection\nQUERY: go toml unmarshaler recursive embedded struct infinite recursion visited"
	ahb07Adv    = "```go\npackage toml\n\nimport (\n\t\"reflect\"\n\t\"testing\"\n)\n\ntype advSelf struct {\n\t*advSelf\n}\n\nfunc TestAdversarialSelfEmbedCycle(t *testing.T) {\n\tplan := buildPlan(reflect.TypeOf(advSelf{}))\n\tif plan == nil {\n\t\tt.Fatal(\"expected non-nil plan\")\n\t}\n}\n```"

	// AHB-08: External gjson empty-string query operator
	ahb08Patch = "```diff\n--- a/gjson.go\n+++ b/gjson.go\n@@ -761,3 +761,3 @@\n-\t\t\t\t\tif len(value) > 2 && value[0] == '\"' &&\n+\t\t\t\t\tif len(value) >= 2 && value[0] == '\"' &&\n \t\t\t\t\t\tvalue[len(value)-1] == '\"' {\n```\n[TARGET_FILE]gjson.go[/TARGET_FILE]"
	ahb08Triage = "ERROR_LINE: len(value) > 2\nCAUSE: array-path parser rejects the empty quoted string\nQUERY: gjson empty string query operator"
	ahb08Adv    = "```go\npackage gjson\n\nimport \"testing\"\n\nfunc TestAdversarialEmptyQueryMock(t *testing.T) {\n\tif Get(`[\"a\",\"\"]`, `#(!=\"\")#`).Raw != `[\"a\"]` {\n\t\tt.Fatal(\"empty-string filter mismatch\")\n\t}\n}\n```"

	// AHB-09: External golang-jwt/jwt invalid aud claim type
	ahb09Patch = "```diff\n--- a/map_claims.go\n+++ b/map_claims.go\n@@ -77,6 +77,10 @@\n \t\t\t}\n \t\t\tcs = append(cs, vs)\n \t\t}\n+\tcase nil:\n+\t\treturn nil, nil\n+\tdefault:\n+\t\treturn nil, newError(fmt.Sprintf(\"%s is invalid\", key), ErrInvalidType)\n \t}\n \n \treturn cs, nil\n```\n[TARGET_FILE]map_claims.go[/TARGET_FILE]"
	ahb09Triage = "ERROR_LINE: return cs, nil\nCAUSE: parseClaimsString silently ignores non-string and non-slice claim types instead of returning ErrInvalidType\nQUERY: golang-jwt map_claims aud claim type ErrInvalidType"
	ahb09Adv    = "```go\npackage jwt\n\nimport (\n\t\"errors\"\n\t\"testing\"\n)\n\nfunc TestAdversarialAudienceNumeric(t *testing.T) {\n\tm := MapClaims{\"aud\": 12345}\n\t_, err := m.GetAudience()\n\tif !errors.Is(err, ErrInvalidType) {\n\t\tt.Fatalf(\"expected ErrInvalidType for integer aud, got %v\", err)\n\t}\n}\n```"
)

// StartServer launches a pure Go mock server on a random localhost port.
// Every response carries the X-Nemotron-Healer: MOCK header.
func StartServer() *Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Nemotron-Healer", "MOCK")
		path := r.URL.Path

		if r.Method == http.MethodGet && (path == "/" || path == "/v1" || path == "/models" || path == "/v1/models") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"nvidia/Nemotron-3-Ultra-550b-a55b"},{"id":"nvidia/Nemotron-3-Nano-30B-A3B"}]}`))
			return
		}

		if r.Method == http.MethodPost && (path == "/search" || path == "/v1/search") {
			var searchReq struct {
				Query string `json:"query"`
			}
			_ = json.NewDecoder(r.Body).Decode(&searchReq)
			q := strings.ToLower(searchReq.Query)

			answer := "Documentation retrieved for autonomous self-healing grounding."
			title := "Official Documentation"
			url := "https://docs.example.com"
			content := "Official reference specification and API contract documentation."

			if strings.Contains(q, "gjson") {
				answer = "GJSON path syntax supports empty string queries and comparisons via the array-path operator."
				title = "tidwall/gjson Documentation"
				url = "https://github.com/tidwall/gjson"
				content = "GJSON path syntax supports empty string queries and comparisons."
			} else if strings.Contains(q, "jwt") || strings.Contains(q, "claims") || strings.Contains(q, "aud") {
				answer = "JWT MapClaims requires strict claim type validation returning ErrInvalidType on mismatched schemas."
				title = "golang-jwt Documentation"
				url = "https://github.com/golang-jwt/jwt"
				content = "MapClaims parseClaimsString validates aud string or string slice, returning ErrInvalidType otherwise."
			} else if strings.Contains(q, "toml") || strings.Contains(q, "recursive") {
				answer = "pelletier/go-toml uses cycle detection to prevent unbounded recursion on embedded struct definitions."
				title = "pelletier/go-toml Documentation"
				url = "https://github.com/pelletier/go-toml"
				content = "Unmarshaler tracks visited struct types to break recursion cycles on embedded types."
			} else if strings.Contains(q, "diff") || strings.Contains(q, "linehash") {
				answer = "sergi/go-diff shares lineHash across both input texts to preserve line indices."
				title = "sergi/go-diff Documentation"
				url = "https://github.com/sergi/go-diff"
				content = "diffLinesToStrings shares a single lineHash between text1 and text2."
			} else if strings.Contains(q, "pydantic") {
				answer = "Pydantic v2 replaces @validator with @field_validator and Config class with model_config = ConfigDict."
				title = "Pydantic v2 Migration Guide"
				url = "https://docs.pydantic.dev/latest/migration/"
				content = "Use @field_validator and ConfigDict(from_attributes=True) in Pydantic v2."
			} else if strings.Contains(q, "sql") {
				answer = "Use parameterized queries (? or :param) to prevent SQL injection vulnerabilities in SQLite."
				title = "Python sqlite3 Documentation"
				url = "https://docs.python.org/3/library/sqlite3.html"
				content = "Always use parameterized queries cursor.execute(query, params) instead of string formatting."
			} else if strings.Contains(q, "asyncio") || strings.Contains(q, "lock") || strings.Contains(q, "balance") {
				answer = "Use synchronization primitives (threading.Lock or asyncio.Lock) to serialize concurrent access to shared mutable state."
				title = "Async Concurrency Documentation"
				url = "https://docs.python.org/3/library/asyncio-sync.html"
				content = "Protect shared state mutations across concurrent coroutines using atomic locks."
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]interface{}{
				"answer": answer,
				"results": []map[string]interface{}{
					{
						"title":   title,
						"url":     url,
						"content": content,
						"score":   0.98,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == http.MethodPost && (path == "/extract" || path == "/v1/extract") {
			var extReq struct {
				URLs []string `json:"urls"`
				URL  string   `json:"url"`
			}
			_ = json.NewDecoder(r.Body).Decode(&extReq)
			targetURL := extReq.URL
			if targetURL == "" && len(extReq.URLs) > 0 {
				targetURL = extReq.URLs[0]
			}
			if targetURL == "" {
				targetURL = "https://docs.example.com"
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"url":         targetURL,
						"raw_content": fmt.Sprintf("[MOCK EXTRACT] Official specifications and architectural invariants for %s.", targetURL),
					},
				},
				"failed_results": []interface{}{},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == http.MethodPost && (path == "/chat/completions" || path == "/v1/chat/completions") {
			var chatReq struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
				Stream bool `json:"stream"`
			}
			_ = json.NewDecoder(r.Body).Decode(&chatReq)

			allContent := ""
			for _, m := range chatReq.Messages {
				allContent += " " + m.Content
			}
			lower := strings.ToLower(allContent)

			isTriage := strings.Contains(lower, "triage") || strings.Contains(allContent, "ERROR_LINE") || strings.Contains(allContent, "4-word")
			isAdversarial := strings.Contains(lower, "adversarial") || strings.Contains(allContent, "Red-Teamer") || strings.Contains(lower, "stress")

			var reply string

			// Case matching based on codebase symbols and files in prompt
			switch {
			case strings.Contains(allContent, "diffmatchpatch") || strings.Contains(allContent, "diffLinesToStrings") || strings.Contains(allContent, "lineHash") || strings.Contains(allContent, "TestDiffLinesToChars"):
				if isTriage {
					reply = ahb06Triage
				} else if isAdversarial {
					reply = ahb06Adv
				} else {
					reply = ahb06Patch
				}

			case strings.Contains(allContent, "unmarshaler.go") || strings.Contains(allContent, "TestUnmarshalRecursiveEmbedded") || strings.Contains(allContent, "addFields") || strings.Contains(allContent, "structPlan"):
				if isTriage {
					reply = ahb07Triage
				} else if isAdversarial {
					reply = ahb07Adv
				} else {
					reply = ahb07Patch
				}

			case strings.Contains(allContent, "map_claims.go") || strings.Contains(allContent, "TestMapClaims_GetAudience") || strings.Contains(allContent, "parseClaimsString"):
				if isTriage {
					reply = ahb09Triage
				} else if isAdversarial {
					reply = ahb09Adv
				} else {
					reply = ahb09Patch
				}

			case strings.Contains(allContent, "cache.go") || strings.Contains(allContent, "cache_test.go") || strings.Contains(allContent, "MetricCache") || strings.Contains(allContent, "TestConcurrentMetricAccess"):
				if isTriage {
					reply = ahb05Triage
				} else if isAdversarial {
					reply = ahb05Adv
				} else {
					reply = ahb05Patch
				}

			case strings.Contains(allContent, "service.py") || strings.Contains(allContent, "test_cascade.py") || strings.Contains(allContent, "BankingService"):
				if isTriage {
					reply = ahb02Triage
				} else if isAdversarial {
					reply = ahb02Adv
				} else {
					reply = ahb02Patch
				}

			case strings.Contains(allContent, "account.py") || strings.Contains(allContent, "test_account.py") || strings.Contains(allContent, "BankLedger"):
				if isTriage {
					reply = ahb01Triage
				} else if isAdversarial {
					reply = ahb01Adv
				} else {
					reply = ahb01Patch
				}

			case strings.Contains(allContent, "models.py") || strings.Contains(allContent, "test_models.py") || strings.Contains(allContent, "from pydantic"):
				if isTriage {
					reply = ahb03Triage
				} else if isAdversarial {
					reply = ahb03Adv
				} else {
					reply = ahb03Patch
				}

			case strings.Contains(allContent, "repo.py") || strings.Contains(allContent, "test_repo.py") || strings.Contains(allContent, "UserRepository"):
				if isTriage {
					reply = ahb04Triage
				} else if isAdversarial {
					reply = ahb04Adv
				} else {
					reply = ahb04Patch
				}

			default:
				// AHB-08 external_gjson default
				isGrounded := strings.Contains(allContent, "GJSON path syntax supports empty string")
				if isTriage {
					reply = ahb08Triage
				} else if isAdversarial {
					reply = ahb08Adv
				} else if !isGrounded && strings.Contains(allContent, "DisableGrounding") {
					reply = "```diff\n--- a/gjson.go\n+++ b/gjson.go\n@@ -761,7 +761,7 @@\n-\t\t\t\t\tif len(value) > 2 && value[0] == '\"' &&\n+\t\t\t\t\tif len(value) > 5 && value[0] == '\"' &&\n \t\t\t\t\t\tvalue[len(value)-1] == '\"' {\n```\n[TARGET_FILE]gjson.go[/TARGET_FILE]"
				} else {
					reply = ahb08Patch
				}
			}

			if chatReq.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.WriteHeader(http.StatusOK)
				flusher, _ := w.(http.Flusher)

				chunk := map[string]interface{}{
					"choices": []map[string]interface{}{
						{"delta": map[string]string{"content": reply}},
					},
					"usage": map[string]int{
						"prompt_tokens":     120,
						"completion_tokens": 45,
					},
				}
				d, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", d)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			respObj := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"content": reply}},
				},
				"usage": map[string]int{
					"prompt_tokens":     120,
					"completion_tokens": 45,
				},
			}
			_ = json.NewEncoder(w).Encode(respObj)
			return
		}

		http.NotFound(w, r)
	}

	ts := httptest.NewServer(http.HandlerFunc(handler))
	return &Server{Server: ts}
}

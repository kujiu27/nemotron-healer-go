#!/usr/bin/env python3
"""Labeled mock of Nebius Token Factory + Tavily for offline pipeline smoke.

Serves OpenAI-compatible endpoints on 127.0.0.1:
  POST /v1/chat/completions  (stream:false JSON and SSE stream:true)
  POST /search, POST /extract (Tavily shapes)

Supports all 9 AHB benchmark cases with verifiable ground-truth patches,
triage diagnostics, and adversarial counter-examples.
The server header marks X-Nemotron-Healer: MOCK on every response.
"""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CASES = {
    "ahb01": {
        "patch": """```diff
--- a/account.py
+++ b/account.py
@@ -1,21 +1,25 @@
 \"\"\"Concurrent banking transaction service suffering from an async race condition.\"\"\"
 
 import asyncio
+import threading
 from typing import Dict
 
 
 class BankLedger:
     def __init__(self, initial_balance: float = 1000.0):
         self.balance = initial_balance
+        self._lock = threading.Lock()
 
     async def transfer(self, amount: float):
         \"\"\"Simulate async transfer with non-atomic state mutation (Race Condition).\"\"\"
-        current = self.balance
-        # Simulate I/O latency exposing race condition
-        await asyncio.sleep(0.001)
-        self.balance = current - amount
+        with self._lock:
+            current = self.balance
+            # Simulate I/O latency exposing race condition
+            await asyncio.sleep(0.001)
+            self.balance = current - amount
 
     async def deposit(self, amount: float):
-        current = self.balance
-        await asyncio.sleep(0.001)
-        self.balance = current + amount
+        with self._lock:
+            current = self.balance
+            await asyncio.sleep(0.001)
+            self.balance = current + amount
```
[TARGET_FILE]account.py[/TARGET_FILE]""",
        "triage": "ERROR_LINE: assert ledger.balance == 0.0\nCAUSE: concurrent transfer and deposit coroutines mutate balance without atomic synchronization\nQUERY: python asyncio shared balance synchronization lock",
        "adv": """```python
import asyncio
from account import BankLedger

def test_adversarial_balance_invariants():
    ledger = BankLedger(100.0)
    async def _run():
        await asyncio.gather(ledger.transfer(50.0), ledger.deposit(50.0))
        assert ledger.balance == 100.0
    asyncio.run(_run())
```"""
    },
    "ahb02": {
        "patch": """```diff
--- a/service.py
+++ b/service.py
@@ -10,7 +10,8 @@
         self.engine = LedgerEngine(initial_balance=initial_balance)
 
     async def single_transfer(self, account_id: str, amount: float):
-        return await self.engine.mutate_balance(account_id, -amount)
+        async with self.engine.lock:
+            return await self.engine.mutate_balance(account_id, -amount)
 
     async def atomic_batch_transfer(self, account_id: str, amounts: List[float]):
```
[TARGET_FILE]service.py[/TARGET_FILE]""",
        "triage": "ERROR_LINE: assert srv.engine.balance == 1000.0\nCAUSE: single_transfer mutates balance without acquiring transaction lock\nQUERY: python asyncio shared lock state synchronization",
        "adv": """```python
import asyncio
from service import BankingService

def test_adversarial_concurrency_stress():
    async def _run():
        srv = BankingService(initial_balance=5000.0)
        tasks = [srv.single_transfer("acc_stress", 50.0) for _ in range(20)]
        await asyncio.gather(*tasks)
        assert srv.engine.balance == 4000.0
    asyncio.run(_run())
```"""
    },
    "ahb03": {
        "patch": """```diff
--- a/models.py
+++ b/models.py
@@ -1,7 +1,7 @@
 \"\"\"Sample application models written with legacy Pydantic v1 syntax.\"\"\"
 
 from typing import List, Optional
-from pydantic import BaseModel, validator
+from pydantic import BaseModel, ConfigDict, field_validator
 
 
 class Item(BaseModel):
@@ -10,10 +10,10 @@
     price: float
     tags: List[str] = []
 
-    class Config:
-        orm_mode = True
+    model_config = ConfigDict(from_attributes=True)
 
-    @validator("price")
+    @field_validator("price")
+    @classmethod
     def check_price(cls, v):
         if v <= 0:
             raise ValueError("Price must be positive")
@@ -27,4 +27,7 @@
 
     @classmethod
     def from_raw(cls, data: dict):
-        return cls.parse_obj(data)
+        return cls.model_validate(data)
+
+    def to_dict(self) -> dict:
+        return self.model_dump()
```
[TARGET_FILE]models.py[/TARGET_FILE]""",
        "triage": "ERROR_LINE: @validator(\"price\")\nCAUSE: pydantic v1 validator and Config.orm_mode removed in pydantic v2\nQUERY: pydantic v2 migration validator to field_validator ConfigDict",
        "adv": """```python
import pytest
from models import Item

def test_adversarial_negative_price():
    with pytest.raises(ValueError):
        Item(name="invalid", price=-10.0)
```"""
    },
    "ahb04": {
        "patch": """```diff
--- a/repo.py
+++ b/repo.py
@@ -17,9 +17,9 @@
 
     def get_user_by_username(self, username: str) -> Optional[Dict]:
         # VULNERABLE: Direct string formatting vulnerable to SQL Injection
-        query = f"SELECT id, username, role FROM users WHERE username = '{username}'"
+        query = "SELECT id, username, role FROM users WHERE username = ?"
         cursor = self.conn.cursor()
-        cursor.execute(query)
+        cursor.execute(query, (username,))
         row = cursor.fetchone()
         if row:
             return {"id": row[0], "username": row[1], "role": row[2]}
```
[TARGET_FILE]repo.py[/TARGET_FILE]""",
        "triage": "ERROR_LINE: cursor.execute(query)\nCAUSE: raw f-string SQL query vulnerable to SQL injection bypass\nQUERY: sqlite3 parameterized query placeholder sql injection prevention",
        "adv": """```python
from repo import UserRepository

def test_adversarial_sql_injection_probe():
    repo = UserRepository()
    assert repo.get_user_by_username("admin'--") is None
```"""
    },
    "ahb05": {
        "patch": """```diff
--- a/cache.go
+++ b/cache.go
@@ -1,8 +1,12 @@
 package cache
 
-import "time"
+import (
+	"sync"
+	"time"
+)
 
 type MetricCache struct {
+	mu   sync.RWMutex
 	data map[string]int
 }
 
@@ -14,11 +18,15 @@
 
 // Record updates counter without proper synchronization, causing Go runtime data race.
 func (c *MetricCache) Record(key string, val int) {
+	c.mu.Lock()
+	defer c.mu.Unlock()
 	current := c.data[key]
 	time.Sleep(100 * time.Microsecond)
 	c.data[key] = current + val
 }
 
 func (c *MetricCache) Get(key string) int {
+	c.mu.RLock()
+	defer c.mu.RUnlock()
 	return c.data[key]
 }
```
[TARGET_FILE]cache.go[/TARGET_FILE]""",
        "triage": "ERROR_LINE: current := c.data[key]\nCAUSE: unsynchronized concurrent read and write to standard Go map causes fatal runtime data race\nQUERY: go concurrent map data race sync.RWMutex",
        "adv": """```go
package cache

import (
	"sync"
	"testing"
)

func TestAdversarialConcurrentStress(t *testing.T) {
	c := NewMetricCache()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			c.Record("key", v)
			_ = c.Get("key")
		}(i)
	}
	wg.Wait()
}
```"""
    },
    "ahb06": {
        "patch": """```diff
--- a/diffmatchpatch/diff.go
+++ b/diffmatchpatch/diff.go
@@ -1312,18 +1312,18 @@
 func (dmp *DiffMatchPatch) diffLinesToStrings(text1, text2 string) (string, string, []string) {
 	// '\\x00' is a valid character, but various debuggers don't like it. So we'll insert a junk entry to avoid generating a null character.
 	lineArray := []string{""} // e.g. lineArray[4] == 'Hello\\n'
+	lineHash := map[string]int{}
 
 	//Each string has the index of lineArray which it points to
-	strIndexArray1 := dmp.diffLinesToStringsMunge(text1, &lineArray)
-	strIndexArray2 := dmp.diffLinesToStringsMunge(text2, &lineArray)
+	strIndexArray1 := dmp.diffLinesToStringsMunge(text1, &lineArray, lineHash)
+	strIndexArray2 := dmp.diffLinesToStringsMunge(text2, &lineArray, lineHash)
 
 	return intArrayToString(strIndexArray1), intArrayToString(strIndexArray2), lineArray
 }
 
 // diffLinesToStringsMunge splits a text into an array of strings, and reduces the texts to a []string.
-func (dmp *DiffMatchPatch) diffLinesToStringsMunge(text string, lineArray *[]string) []uint32 {
+func (dmp *DiffMatchPatch) diffLinesToStringsMunge(text string, lineArray *[]string, lineHash map[string]int) []uint32 {
 	// Walk the text, pulling out a substring for each line. text.split('\\n') would would temporarily double our memory footprint. Modifying text would create many large strings to garbage collect.
-	lineHash := map[string]int{} // e.g. lineHash['Hello\\n'] == 4
 	lineStart := 0
 	lineEnd := -1
 	strs := []uint32{}
```
[TARGET_FILE]diffmatchpatch/diff.go[/TARGET_FILE]""",
        "triage": "ERROR_LINE: diffLinesToStringsMunge(text2, &lineArray)\nCAUSE: local lineHash map not shared between text1 and text2 causes mismatched line indices\nQUERY: sergi go-diff lineHash diffLinesToStringsMunge share index",
        "adv": """```go
package diffmatchpatch

import "testing"

func TestAdversarialLineHashIdentical(t *testing.T) {
	dmp := New()
	a, b, lines := dmp.diffLinesToStrings("hello\\n", "hello\\n")
	if a != b || len(lines) != 2 {
		t.Fatal("line hash sharing failure")
	}
}
```"""
    },
    "ahb07": {
        "patch": """```diff
--- a/unmarshaler.go
+++ b/unmarshaler.go
@@ -2260,11 +2260,17 @@
 		byName: map[string]structField{},
 		byFold: map[string]structField{},
 	}
-	addFields(plan, t, nil)
+	visited := map[reflect.Type]bool{}
+	addFields(plan, t, nil, visited)
 	return plan
 }
 
-func addFields(plan *structPlan, t reflect.Type, prefix []int) {
+func addFields(plan *structPlan, t reflect.Type, prefix []int, visited map[reflect.Type]bool) {
+	if visited[t] {
+		return
+	}
+	visited[t] = true
+	defer func() { delete(visited, t) }()
 	var embedded []reflect.StructField
 	for i := 0; i < t.NumField(); i++ {
 		f := t.Field(i)
@@ -2344,7 +2350,7 @@
 		index = append(index, prefix...)
 		idx := f.Index[0]
 		index = append(index, idx)
-		addFields(plan, ft, index)
+		addFields(plan, ft, index, visited)
 	}
 }
```
[TARGET_FILE]unmarshaler.go[/TARGET_FILE]""",
        "triage": "ERROR_LINE: addFields(plan, ft, index)\nCAUSE: unmarshaling plan enters infinite recursive loop on mutually or self-embedded structs without cycle detection\nQUERY: go toml unmarshaler recursive embedded struct infinite recursion visited",
        "adv": """```go
package toml

import (
	"reflect"
	"testing"
)

type advSelf struct {
	*advSelf
}

func TestAdversarialSelfEmbedCycle(t *testing.T) {
	plan := buildPlan(reflect.TypeOf(advSelf{}))
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
}
```"""
    },
    "ahb08": {
        "patch": """```diff
--- a/gjson.go
+++ b/gjson.go
@@ -761,3 +761,3 @@
-					if len(value) > 2 && value[0] == '"' &&
+					if len(value) >= 2 && value[0] == '"' &&
 						value[len(value)-1] == '"' {
```
[TARGET_FILE]gjson.go[/TARGET_FILE]""",
        "triage": "ERROR_LINE: len(value) > 2\nCAUSE: array-path parser rejects the empty quoted string\nQUERY: gjson empty string query operator",
        "adv": """```go
package gjson

import "testing"

func TestAdversarialEmptyQueryMock(t *testing.T) {
	if Get(`["a",""]`, `#(!="")#`).Raw != `["a"]` {
		t.Fatal("empty-string filter mismatch")
	}
}
```"""
    },
    "ahb09": {
        "patch": """```diff
--- a/map_claims.go
+++ b/map_claims.go
@@ -77,6 +77,10 @@
 			}
 			cs = append(cs, vs)
 		}
+	case nil:
+		return nil, nil
+	default:
+		return nil, newError(fmt.Sprintf("%s is invalid", key), ErrInvalidType)
 	}
 
 	return cs, nil
```
[TARGET_FILE]map_claims.go[/TARGET_FILE]""",
        "triage": "ERROR_LINE: return cs, nil\nCAUSE: parseClaimsString silently ignores non-string and non-slice claim types instead of returning ErrInvalidType\nQUERY: golang-jwt map_claims aud claim type ErrInvalidType",
        "adv": """```go
package jwt

import (
	"errors"
	"testing"
)

func TestAdversarialAudienceNumeric(t *testing.T) {
	m := MapClaims{"aud": 12345}
	_, err := m.GetAudience()
	if !errors.Is(err, ErrInvalidType) {
		t.Fatalf("expected ErrInvalidType for integer aud, got %v", err)
	}
}
```"""
    }
}


def reply_for(req):
    msgs = req.get("messages", [])
    sys = " ".join(m.get("content", "") for m in msgs if m.get("role") == "system")
    user = " ".join(m.get("content", "") for m in msgs if m.get("role") == "user")
    all_content = sys + " " + user
    sys_lower = sys.lower()

    is_triage = "triage" in sys_lower or "error_line" in user or "4-word" in user
    is_adv = "adversarial" in sys_lower or "red-team" in sys_lower

    matched = "ahb08"
    if "diffmatchpatch" in all_content or "difflinestostrings" in all_content or "linehash" in all_content:
        matched = "ahb06"
    elif "unmarshaler.go" in all_content or "testunmarshalrecursiveembedded" in all_content or "addfields" in all_content:
        matched = "ahb07"
    elif "map_claims.go" in all_content or "testmapclaims_getaudience" in all_content or "parseclaimsstring" in all_content:
        matched = "ahb09"
    elif "cache.go" in all_content or "cache_test.go" in all_content or "metriccache" in all_content:
        matched = "ahb05"
    elif "service.py" in all_content or "test_cascade.py" in all_content or "bankingservice" in all_content:
        matched = "ahb02"
    elif "account.py" in all_content or "test_account.py" in all_content or "bankledger" in all_content:
        matched = "ahb01"
    elif "models.py" in all_content or "test_models.py" in all_content or "from pydantic" in all_content:
        matched = "ahb03"
    elif "repo.py" in all_content or "test_repo.py" in all_content or "userrepository" in all_content:
        matched = "ahb04"

    entry = CASES[matched]
    if is_triage:
        return entry["triage"]
    if is_adv:
        return entry["adv"]
    return entry["patch"]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def _send(self, obj, status=200):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("X-Nemotron-Healer", "MOCK")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        req = json.loads(self.rfile.read(n) or b"{}")

        if self.path == "/search" or self.path == "/v1/search":
            q = req.get("query", "").lower()
            ans = "[MOCK] Documentation retrieved for autonomous self-healing grounding."
            title = "[MOCK] Official Documentation"
            url = "https://docs.example.com"
            content = "Official reference specification and API contract documentation."

            if "gjson" in q:
                ans = "GJSON path syntax supports empty string queries and comparisons via the array-path operator."
                title = "tidwall/gjson Documentation"
                url = "https://github.com/tidwall/gjson"
                content = "GJSON path syntax supports empty string queries and comparisons."
            elif "jwt" in q or "claims" in q or "aud" in q:
                ans = "JWT MapClaims requires strict claim type validation returning ErrInvalidType on mismatched schemas."
                title = "golang-jwt Documentation"
                url = "https://github.com/golang-jwt/jwt"
                content = "MapClaims parseClaimsString validates aud string or string slice, returning ErrInvalidType otherwise."
            elif "toml" in q or "recursive" in q:
                ans = "pelletier/go-toml uses cycle detection to prevent unbounded recursion on embedded struct definitions."
                title = "pelletier/go-toml Documentation"
                url = "https://github.com/pelletier/go-toml"
                content = "Unmarshaler tracks visited struct types to break recursion cycles on embedded types."
            elif "diff" in q or "linehash" in q:
                ans = "sergi/go-diff shares lineHash across both input texts to preserve line indices."
                title = "sergi/go-diff Documentation"
                url = "https://github.com/sergi/go-diff"
                content = "diffLinesToStrings shares a single lineHash between text1 and text2."

            return self._send({
                "query": req.get("query", ""),
                "answer": ans,
                "results": [{"title": title, "url": url, "content": content, "score": 0.99}]
            })

        if self.path == "/extract" or self.path == "/v1/extract":
            url = (req.get("urls") or [req.get("url", "")])[0]
            if not url:
                url = "https://docs.example.com"
            return self._send({
                "results": [{"url": url, "raw_content": f"[MOCK EXTRACT] Architectural invariants for {url}."}],
                "failed_results": []
            })

        if self.path.endswith("/chat/completions"):
            text = reply_for(req)
            if req.get("stream"):
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("X-Nemotron-Healer", "MOCK")
                self.end_headers()
                chunk = {"choices": [{"delta": {"content": text}}]}
                self.wfile.write(f"data: {json.dumps(chunk)}\n\n".encode())
                self.wfile.write(b"data: [DONE]\n\n")
                return
            return self._send({
                "choices": [{"message": {"role": "assistant", "content": text}}],
                "usage": {"prompt_tokens": 111, "completion_tokens": 222}
            })
        self._send({"error": "unknown path"}, 404)


if __name__ == "__main__":
    port = int(__import__("sys").argv[1]) if len(__import__("sys").argv) > 1 else 18080
    print(f"MOCK Nebius/Tavily on http://127.0.0.1:{port} (X-Nemotron-Healer: MOCK)")
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()

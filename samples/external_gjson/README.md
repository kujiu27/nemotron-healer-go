# External case: tidwall/gjson empty-string query operator (reverted fix)

Real third-party historical bug, not authored for this project.

- Upstream fix commit: https://github.com/tidwall/gjson/commit/0b52f9a4c6d94a0d90f8b5f7b3a8fa0e69e1e1c8
  "Fix empty string operator not matching" (issue #246)
- Tree state: parent of the fix commit (`0b52f9a~1`) PLUS the regression
  test case added by that commit (`TestEmptyValueQuery`).
- Red state: `go test . -run TestEmptyValueQuery -count=1` fails — the
  array-path parser required `len(value) > 2` before stripping quotes, so a
  query comparing against the EMPTY string (`#(!="")#`) was mis-parsed and
  returned wrong results.
- Ground-truth solution: the upstream one-line fix
  (`len(value) > 2` → `len(value) >= 2` in `parseArrayPath`).
- License: MIT (upstream). See LICENSE in this directory.

Verified locally: buggy tree FAILs the case; applying the upstream fix makes
it PASS. No `.orig` answer files shipped.

# External case: golang-jwt/jwt v5 invalid `aud` claim type (reverted fix)

Real third-party historical bug, not authored for this project.

- Upstream fix commit: https://github.com/golang-jwt/jwt/commit/1a11d37d3d83f64cf0c6059f2b1e02b6f5a2eca3
  "fix: return ErrInvalidType for an invalid aud claim type in MapClaims" (#511)
- Tree state: parent of the fix commit (`1a11d37~1`) PLUS the regression
  test case added by that commit (`TestMapClaims_GetAudience`).
- Red state: `go test . -run TestMapClaims_GetAudience -count=1` fails —
  `MapClaims.parseClaimsString` silently accepted wrong-typed claims, so
  `GetAudience()` returned no error for `aud` values like numbers, booleans
  or objects instead of `ErrInvalidType` (inconsistent with every other
  claim accessor — a JWT-validation correctness defect).
- Ground-truth solution: the upstream 10-line fix (a `nil` case plus a
  `default:` case returning `ErrInvalidType` in `parseClaimsString`).
- License: MIT (upstream). See LICENSE in this directory.

Verified locally: buggy tree FAILs the case; applying the upstream fix makes
it PASS. No `.orig` answer files shipped.

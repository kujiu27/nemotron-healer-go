# External case: sergi/go-diff lineHash bug (reverted fix)

Real third-party historical bug, not authored for this project.

- Upstream fix commit: https://github.com/sergi/go-diff/commit/6dbe13c91e8e382b53e2cc8e8146fa6172fefec4
  "fix: use common lineHash to share indice between text1 and text2"
- Tree state: parent of the fix commit (`6dbe13c~1`) PLUS the regression
  test case added by that commit.
- Red state: `go test ./diffmatchpatch/ -run TestDiffLinesToChars` fails on
  "Same lines in Text1 and Text2" (line indices wrong when identical lines
  appear in both texts — per-text lineHash instead of a shared one).
- Ground-truth solution: the upstream commit itself (share one lineHash across
  both texts in `diffLinesToStrings`).
- License: Apache-2.0 (upstream). See LICENSE in this directory.

Verified locally: buggy code FAILs the case; applying the upstream fix makes
it PASS. So the healing target is reachable and minimal.

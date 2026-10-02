# External case: pelletier/go-toml recursive-embedded infinite recursion (reverted fix)

Real third-party historical bug, not authored for this project.

- Upstream fix commit: https://github.com/pelletier/go-toml/v2/commit/6fa69af927671a92f97a3e5a74e9a533e39c19f8
  "fix: do not recurse forever on recursively embedded structs (#1087)"
- Tree state: parent of the fix commit (`6fa69af~1`) PLUS the regression
  test case added by that commit (`TestUnmarshalRecursiveEmbedded`).
- Red state: `go test . -run TestUnmarshalRecursiveEmbedded -count=1` fails —
  unmarshaling into a struct that embeds itself (directly or via a mutual
  pair) sends `buildPlan`/`addFields` into infinite recursion; the test
  process hangs and is killed (Go stack keeps growing, no termination).
- Ground-truth solution: the upstream commit itself (thread a
  `visited map[reflect.Type]bool` through `addFields` and stop descending
  into types already on the current branch).
- License: MIT (upstream). See LICENSE in this directory.

Verified locally: buggy code FAILs the case (hang → kill, ~20s); applying the
upstream fix makes it PASS in ~0.5s. No `.orig` answer files shipped.

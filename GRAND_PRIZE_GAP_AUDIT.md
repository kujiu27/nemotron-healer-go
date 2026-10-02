# 大奖差距审计:最挑剔评委 10 轮拷问

日期:2026-10-01。基准:Nebius x NVIDIA Global AI Hackathon(Devpost)。
> **状态(2026-10-02)**:本文档为初始审计快照,保留原样以供对账。全部代码级发现在后续 7 轮迭代中
> 已修复 —— 见文末「修复对账账本」。当前仍开放项仅有两项人力项:真实 API eval 留痕、视频录制。
硬性门槛:① 运行于 Nebius Token Factory 或 Nebius AI Cloud;② 至少用一个 NVIDIA 开源模型(Nemotron/GR00T/Cosmos/Sonic)。
提交物:可运行 demo、3 分钟视频、开源许可公开仓库、项目描述。截止:2026-10-30 10:00 PT。
奖金:大奖 $20k / 二等 $10k / 三等 $6k / Best Use of Tavily $3k。注册人数 13,148(2026-09-28)。
赛道选择:Coding and Agentic Engineering。

每轮均给出仓库证据(文件:行)。结论按"致命 / 重伤 / 轻伤"分级。

---

## 第 1 轮:资格拷问 — "真的跑在 Nebius 上吗?证据在哪?"

**代码层面合规**:`internal/client/nebius.go:94-95` 默认 `https://api.tokenfactory.nebius.com/v1`,默认模型 `nvidia/nemotron-3-ultra` + `nvidia/nemotron-mini-4b`。

**但评委只看得到提交物,看不到你的意图**:
- 全仓库无任何一次真实 API 调用的留痕:无 eval 输出、无 session log、无 benchmark 结果文件、无 token ledger 截图。
- 14 个单测全部 mock/本地,无一个集成测试打真 Token Factory。
- `nemotron-mini-4b`(FastModel)在 NVIDIA NIM 已标注 2026-08-25 弃用;且 grep 显示 FastModel 除 doctor 打印外**零处用于推理**——死配置。评委追问"双模型架构"当场穿帮。

**等级:重伤。** 修法:跑一次真实 `eval`,把完整输出连同 token ledger 提交进仓库(如 `docs/EVAL_RESULTS.md`);删 FastModel 或真用起来。

---

## 第 2 轮:评测拷问 — "AHB-4 是谁认证的 official?"

`internal/cli/eval.go:31` 自称 "Official Autonomous Healer Benchmark (AHB-4)"。**official 是自封的**。4 个 case 全是自写 `samples/`,出题人=解题人。samples 里还留着 `engine.py.orig`、`account.py.orig`——答案就躺在仓库里。

- 无任何外部基准:SWE-bench Lite、Defects4J、甚至从 GitHub issues 抓的真失败 repo,一个都没有。
- 消融实验 n=1,无重复、无方差、无随机种子;"+X% Grounding Delta" 由 4 个样本算出,统计上是噪声。
- 消融设计缺陷:baseline 同时关掉 Tavily **和** archetype 约束(`DisableGrounding` 一个开关管两件事),无法归因增益来源。

**等级:重伤。** 修法:接入 10-20 个真实开源 repo 的历史失败用例,或最低限度 5 次重复跑 AHB-4 报均值±方差;拆分消融开关。

---

## 第 3 轮:MCTS 拷问 — "深度为 1 的树也敢叫 MCTS?"

`internal/engine/mcts_search.go`:展开 root 的 N 个假设子节点,各 rollout 一次,`BestChild(1.414)` 选优。`SearchDepth: 1`(mcts_search.go:169 硬编码)。无 selection 迭代、无深层扩展、无失败信息回流(rollout 失败不改变下一个 hypothesis 的生成)。

更难堪的数学:每个子节点只访问 1 次,UCB1 探索项 `sqrt(ln(1)/1) = 0`——**UCB1 在此架构下数学上退化为取最大均值**。懂 ML 的评委一句话点破:"这是 best-of-N 加了戏剧台词。"

**等级:重伤(命名欺诈风险)。** 修法二选一:① 实现真迭代(≥2 层:失败 rollout 的错误输出作为反馈生成第二层子假设);② 诚实改名 "Divergent Hypothesis Search"。评委恨镀金甚于恨简单。

---

## 第 4 轮:对抗验证拷问 — "fail-open 的验证等于没有验证"

`internal/falsify/falsify.go`:读文件失败→`Passed:true`(42 行);LLM 错误→`Passed:true`(68 行);提取空→`Passed:true`(73 行);写文件失败→`Passed:true`(80 行)。全部基础设施错误静默降级为"通过",并附上拍脑袋的 `ConfidenceScore: 0.8/0.85` 常量。审计卡再打印 "Status: PASSED (Confidence: 0.8)"——**凭空捏造的置信度写进对外报告**。

安全工程评委定性:防过拟合门禁在任何抖动时变 no-op,是验证剧场。你的差异化卖点(anti-overfitting)恰恰是最脆的一环。

**等级:致命(对卖点可信度)。** 修法:所有 infra 错误 fail-closed(拒绝该 branch 或重试);置信度要么实测要么删掉。

---

## 第 5 轮:沙箱拷问 — "sh -c 也配叫 Transactional Sandbox?"

`internal/sandbox/runner.go:40`:`exec.CommandContext(ctx, "sh", "-c", cmdStr)`。无容器、无 namespace、无 seccomp、无网络隔离、无内存/CPU 限制、无 env 清洗。LLM 生成的 patch 代码 + falsifier 生成的测试**以宿主全权限、联网、可读环境变量密钥执行**。Tavily 返回的网页片段或 stack trace 里的提示注入,直接变成任意命令执行。

"事务性"也有洞:`checkpoint.go` Rollback 只做 `git checkout .` + 从备份回拷。**patch 新建的文件不会被删除**(备份 walk 里没有它),残留污染下一次 rollout——DESIGN.md 宣称 "Zero repository contamination" 与实现矛盾。

**等级:重伤(诚实性)+ 真实安全洞。** 修法:Rollback 加 `git clean -fd` 或 diff 集合删除新增文件;README 明示执行边界;高配加分项:可选 Docker/gVisor 隔离模式。

---

## 第 6 轮:AST 拷问 — "bufio.Scanner + 正则也配叫 AST Blast Radius?"

`internal/ast/blast_radius.go:97`:`bufio.NewScanner` 逐行正则匹配函数/类定义。全仓库无 `go/ast`、无 `go/parser`、无 tree-sitter(grep 证实)。Go 方法接收者 `func (r *T) M()`、Python 嵌套函数、多行签名、装饰器全部漏解析。DESIGN.md 拿这个图写公式 $R=\min(1, \frac{|TD|+|AF|}{0.5|V|})$——**真数学长在假图上**。

DESIGN 还宣称 R>0.7 时 "restricts unified diffs to non-signature-breaking optimizations"——`internal/mcts/reward.go` 里 blast risk 只占 0.10 权重,**代码中不存在任何限制 diff 签名的逻辑**。宣称的机制没实现。

**等级:重伤。** 修法:Go 侧换 `go/parser`(标准库,白捡),Python 侧 tree-sitter 或至少宣称降级为"符号扫描";R>0.7 约束要么实现要么从 DESIGN 删掉。

---

## 第 7 轮:经济叙事拷问 — "$0.0004 一次修复,收据呢?"

- `internal/engine/fsm.go:35`:成本 = $0.10/1M prompt + $0.30/1M completion。审计卡模板(agent.go:376)却写 "At $0.20/1M tokens"——**代码与报告单价自相矛盾**。Nemotron 3 Ultra(550B/55B-active MoE)在 Token Factory 的真实定价未核对。
- "99.8% 节省" 的分母是虚构的 "$25 人工基线(30min @ $50/hr)"(fsm.go:38)。视频脚本(DEMO_VIDEO_SCRIPT.md)已写死 "$0.0004 per fix"——**这个数字没有任何一次真实运行支撑**。
- README 的 "<5ms 冷启动 / ~14MB RSS / 6.9MB 二进制" 全部可秒测却一个都没测(6.9MB 倒是 dist 里有文件可证)。

评委要一张收据,仓库给不出一张。

**等级:重伤。** 修法:对齐单价到 Token Factory catalog 实价;跑真实 eval 后用实测数字替换脚本;冷启动/RSS 用一条 benchmark 命令留痕。

---

## 第 8 轮:提交物拷问 — "评委先看视频,你视频呢?"

硬性提交物清单 vs 仓库现状:
| 提交物 | 现状 |
|---|---|
| 3 分钟视频 | 只有 `DEMO_VIDEO_SCRIPT.md` 脚本,无 mp4/mov/webm(find 证实) |
| 可运行 demo | `demo/run_pitch_demo.sh` 依赖本地 `./bin/` + python3 + pytest;action.yml 是加分项 ✔ |
| 公开仓库 + 开源许可 | MIT ✔ |
| 项目描述(Devpost) | 仓库无草稿文本 |

距截止 29 天。13,148 人报名,评委人均几十个项目、每个项目几分钟。**当前仓库 100% 工程、0% 提交物**。大奖第一死因从来不是技术,是没人看到技术。

**等级:致命(直接出局型)。** 修法:本周录视频(脚本是全仓库最完整的资产,照录即可);写 Devpost 描述草稿;README 顶部放 30 秒 GIF。

---

## 第 9 轮:Tavily 奖拷问 — "$3k Best Use of Tavily,你想不想要?"

项目已用 Tavily(契合该奖),但深度薄:
- `internal/engine/grounding.go:29-31`:query 是硬编码常量串("pydantic v2 migrate validator to field_validator official docs" 原样写死)。
- 只用 search API,3 条结果直接塞 prompt;无 Extract API 全文抽取、无 rerank、无缓存、无配额重试;错误全部 `_ =` 吞掉(agent.go:240)。
- 亮点:审计卡里的引用表(标题+URL+score+摘录)是真差异化,保留。
- 竞争对手会用 extract+rerank+多轮检索。当前实现是一个薄 wrapper。

**等级:轻伤(单独赛道)。** 修法:对 top 结果加 Extract API 全文;失败时降级且在审计卡如实标注;query 组装用模型生成而非查表。

---

## 第 10 轮:大奖差异化拷问 — "13,148 人里,评委凭什么记住你?"

真优势(守住):Go 单二进制 + GitHub Action 分发故事、审计卡、Red/Blue arena 概念、TUI。
真弱点(会被放大):
- **命名镀金链**:MCTS(实为 depth-1)、"Minimax Self-Play"(arena 跑 2 轮)、"Nash Equilibrium"(agent.go:306 由布尔值宣布)、"Alibaba OCR Hybrid"(借名)。每个词都是评委的靶子。评委对 grandiose label 包着的简单循环的惩罚,远重于对诚实简单系统的宽容。
- **零第三方真实 repo 修复证据**:4 个自造样本撑不起 "in-situ autonomous" 叙事。拿一个真开源项目的历史失败 commit,现场修,录进视频——一个真实例胜过十个 AHB case。
- `internal/engine/agent.go` 415 行单函数文件,审计卡 fmt 模板内嵌 70 行——可维护性扣分项(次要)。

**结论(按夺奖影响排序的真实差距)**:
1. 无提交物(视频/描述)——出局级,29 天内最高优先级。
2. 验证链 fail-open + 回滚残留新文件——卖点即谎言级。
3. 零真实运行留痕、评测自出自解——可信度级。
4. 命名与文档超出实现(MCTS/沙箱/AST/限制承诺)——诚实级,逐条改名或补实现。
5. Tavily 深度薄——$3k 赛道失分。

工程完成度高、差异化真;输给"评委看不见 + 不敢信"的概率远大于输给技术。

---

## 修复对账账本(2026-10-02)

初始审计 + 7 轮追加拷问的全部发现与归宿。commit 为 main 分支短哈希。

| 原始发现 | 等级 | 现状 | 修复 |
|---|---|---|---|
| 无真实 API 调用留痕 | 重伤 | `scripts/run_real_eval.sh` 就绪,**待导出 key 执行**(人力项) | 26a8dfb |
| FastModel 死配置(nemotron-mini-4b → Llama 不在 catalog) | 重伤 | 默认 `nvidia/Nemotron-3-Nano-30B-A3B`(catalog 实证) | faccd24 |
| AHB 自称 official、自出自解 | 重伤 | 更名 in-repo benchmark;AHB-06(sergi/go-diff 6dbe13c)+ AHB-07(pelletier/go-toml 6fa69af)两真实上游 bug,红绿双向实证 | bec5e31, ddac2d5, c84d094 |
| 消融 n=1 无重复 | 重伤 | `eval --repeat N` 支持方差暴露 | 26a8dfb |
| MCTS 深度 1、UCB1 退化 | 重伤 | 深度-2 对抗反馈强化已实现;用户可见命名改 DHS(Divergent Hypothesis Search);RFC_002 加状态横幅 | 8169d3e, da2291e |
| falsify fail-open(Passed:true on infra error) | 致命 | 全部 fail-closed;跳过场景如实 SKIPPED + 原因 | 87ae3e6 |
| 捏造 ConfidenceScore(0.98/0.50 常量) | 致命 | 字段删除;审计卡只报事实(状态/原因/生成测试) | 7f3ee99 |
| `git checkout .` 回滚残留新文件 | 重伤 | checkpoint manifest 精确删除;单测覆盖 | 87ae3e6 + 回归测试 |
| 正则假 AST(blast radius) | 重伤 | Go `go/parser` 真 AST;Python 原生解析 + py_compile 语法门 | d0afb97 |
| R>0.7 限制 diff 签名承诺未实现 | 重伤 | 高风险目标注入 non-signature-breaking 硬约束进 prompt | d0afb97 |
| 成本单价与 catalog 矛盾 | 重伤 | 对齐 $1.00/$3.00(Nemotron-3-Ultra-550b-a55b catalog 实价) | a73650e |
| $0.0004/99.8% 虚构数字 | 重伤 | 全部删除;数字只来自流式 usage、catalog 或标注假设;GPT-4o 过期基线行删除 | 7f3ee99 |
| 冷启动/RSS/体积"measured"无收据 | 重伤 | `make bench` → `docs/PERF.md` 提交收据,README 逐行链接 | ae780f0 |
| README/action.yml 404 安装路径(零 release + module 路径不解析) | 致命 | `go install` 可解析 module 路径;action 源码构建;`v*` tag CI 自动发布;v0.3.0 release 四平台资产实测 200 | a356ef8 |
| CI go-version 1.22 < go.mod 1.26.5 | 重伤 | `stable`,远程 CI 绿 | a356ef8 |
| demo 假 GREEN / clear 非 tty 退死 | 重伤 | 缺 key 拒跑;heal 失败非零退出;clear 加 guard | a356ef8 |
| samples 自带答案(engine.py 已治愈)+ .orig 散落 | 重伤 | AHB-02 复位损坏基线;外部 case 不带 .orig | a356ef8, c84d094 |
| Alibaba OCR / Minimax / Nash 借名链 | 重伤 | 用户可见全部改名(Archetype Rule Engine / attack-defend rounds);内部标识符保留 | da2291e |
| Tavily 仅 search 薄封装、query 硬编码、错误吞掉 | 中伤 | Search+Extract 双 API;Nemotron Nano 生成 query(规则表降级);失败诚实降级 + notify;httptest 覆盖 | faccd24 |
| Tier-3 兜底把全部 hunk 拼一块、多 hunk 必败 | 致命 | 逐 hunk 应用,no-newline 标记处理,5 新单测 | 2f29e08 |
| 沙箱 sh -c 全权限 | 重伤 | 环境凭证过滤(sanitizeEnvironment);README 明示信任边界 | c6a30b0 前后 |
| 无视频/Devpost 描述 | 致命 | Devpost 描述草稿就绪(DEVPOST_SUBMISSION.md);**视频待录**(人力项) | 995d953 |
| README "measured" 性能无收据 | 重伤 | `make bench` → docs/PERF.md 提交收据 | ae780f0 |
| Action 零 runner 执行证据 / 无 path 输入 / README 无 uses 示例 | 致命 | action-smoke 真 runner 每 push 实证;path 输入;README 用法;@v0.4.0 | 58817a0 系 |
| eval --repeat 无聚合统计 | 重伤 | per-case 胜率 + turns/duration/cost 均值±总体标准差,JSON/MD 导出 | 45007c8 |
| Devpost 草稿未携带证据链 | 重伤 | "Evidence you can verify in 60 seconds" 段落 | 76e95f6 |
| 样本被治愈污染基准无复发防护 | 重伤 | 红态完整性门禁:verify_red.sh + CI benchmark-red job + 收据 | 5edd0a2/4d9a242 |
| AHB-07 挂起类在 CI 触发 job 级击杀 | 重伤 | GOMEMLIMIT=256MiB 自困,坏树有界 FAIL,好树 0.6s | 5edd0a2 |

> 后续追加轮(9-13)发现与修复已并入上表;主 README 的 "Evidence & Verification" 一节给全部收据入口。
### Remediation Status (for judges)

This repo carries its own adversarial self-audit (above, 10 interrogation
rounds, 2026-10-01). Every code-level finding was subsequently fixed and
verified — the table maps each finding to its fix commit on `main`. The two
remaining open items are human tasks: a recorded real-API eval run
(`scripts/run_real_eval.sh` → `docs/EVAL_RESULTS.md`) and the 3-minute demo
video. Offline-verifiable evidence committed in-repo: performance receipts
(`docs/PERF.md`, regenerate via `make bench`), red/green proofs for both
real upstream-bug benchmark cases (`samples/external_*/README.md`), and 44
unit tests plus CI on every push.

# LHT-0001 · 验证标准（VSL-v3 层）

> 本文件是 **VSL-v3 验证标准层**在真实产物上的第一份落地。
> 它存在的直接原因：在此之前，仓库里没有任何真实定义块使用 `verify:` 字段 ——
> 校验器（`doccontract/verifylint.go`）处于**空转**状态，
> 「8/8 通过」只说明"没有样本"，不说明"标准被遵守"（CICD-BOUNDARY-001:389）。
>
> 消费方：`doccontract/verifylint_real_test.go`（扫描并 lint 本文件）。

```yaml
owner: coder-agent
verify:
  claim: "对 LHT-0001 的全部 5 个可判定步骤，每一步的冻结观测（frozen.jsonl 的 obs）字段完整；且 C1/C2/C3 三条判据可由第三方用同一份冻结文件复算出与 settlement 相同的结论"
  method: test
  evidence:
    level: E2
    kind: external_artifact
    ref: "e2e/lht0001_test.go#TestLHT0001FirstSettlement"
  threshold: "判据一致 3/3；第三方复算偏差 = 0"
  counterexample: "同一份 frozen.jsonl 被第三方复算出与 settlement 不同的判定；或某一步 obs 缺失导致该步无法判分（覆盖率 < 5/5）"
  verdict_states: [met, unverified, not_met]
  approver: reviewer-agent
  falsifier: "任何一次用同一份冻结文件复算不出相同结论，即可证伪本定义"
```

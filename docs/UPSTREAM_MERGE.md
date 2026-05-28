# 上游合并维护指南（Fork Maintenance）

本仓库是上游项目的 fork，会**长期、反复地合并上游 main**。本文档记录如何把"合并上游"这件事变得低冲突、可重复。

> 适用对象：维护者本人 + Claude。Claude 在执行"合并上游 / 解决冲突"任务时应遵循本文档。

---

## 1. 冲突的根因

历史上的合并冲突几乎都属于**同一种模式**：上游和本 fork 各自往**同一段共享列表**追加了不同条目，导致追加行相邻冲突。典型位置：

| 位置 | 性质 | 说明 |
|------|------|------|
| `backend/ent/*.go`（`client.go` / `mutation.go` / `migrate/schema.go` 等） | **生成代码** | hooks/inters 实体名列表、schema 列表 |
| `backend/internal/repository/wire.go` 的 `ProviderSet` | 手写 | Provider 函数清单 |
| `backend/cmd/server/wire_gen.go` | **生成代码** | wire 生成的 DI 装配 |

这类"列表追加型"冲突**内容上互不排斥**——两边新增的实体/Provider 都该保留，正确解法永远是**取并集**。

---

## 2. 已落地的防护措施

### 2.1 git rerere（已开启）

```bash
git config rerere.enabled true
git config rerere.autoupdate true
```

Git 会记录每次冲突的解决方式，下次出现**相同冲突**时自动套用并加入暂存。对反复出现的"列表追加"冲突极其有效。**无需重复开启**，本仓库已配置。

### 2.2 fork 专属 ProviderSet（已落地）

fork 相对上游新增的 wire Provider **不再直接写进** `wire.go` 的 `ProviderSet`，而是集中到独立文件：

- `backend/internal/repository/wire_fork.go` → `var ForkProviderSet`
- `wire.go` 的 `ProviderSet` 顶部只用 `ForkProviderSet,` 引用一行

这样上游对 `ProviderSet` 的增删与 fork 的新增行**物理隔离**，不再相邻冲突。
wire 会把嵌套 set 展平，生成结果与内联写法**逐字节相同**，DI 图不变。

> **约定**：今后 fork 新增的 Repository Provider 一律加到 `ForkProviderSet`，**不要**写进 `wire.go`。

---

## 3. 合并上游标准流程（Checklist）

```bash
# 0. 干净工作区，切到 fork 分支
git switch feat/javoo
git status   # 确认无未提交改动

# 1. 拉取并合并上游
git fetch origin
git merge origin/main      # rerere 会自动复用历史解决方案

# 2. 若仍有冲突，按"冲突类型"分类处理（见 §4），然后：
git add <已解决文件>

# 3. 生成代码统一重新生成（关键，见 §4.1）
cd backend && make generate

# 4. 验证
go build ./...
go vet ./...
make test            # 或至少跑受影响的包

# 5. 标记 + 提交合并
git add -A
git commit           # 沿用默认 merge message
```

---

## 4. 按冲突类型解决

### 4.1 生成代码冲突 → 永不手改，一律重新生成 ⭐

`backend/ent/**`、`backend/cmd/server/wire_gen.go` 都是**生成产物**。真正的真相源是：

- ent：`backend/ent/schema/*.go`（每个实体一个独立文件，天然不冲突）
- wire：`backend/internal/repository/wire.go` + `wire_fork.go`

因此生成代码冲突的标准解法是**任选一边丢弃、再重新生成**：

```bash
cd backend
git checkout --theirs ent cmd/server/wire_gen.go   # 任选一边，内容无所谓
make generate                                       # 重新生成
git add ent cmd/server/wire_gen.go
```

> 只要两边的 schema 文件都已正确合并（它们互不冲突），重新生成就会自动产出包含双方实体的正确代码。
> **不要**手动去 merge `client.go` 里的实体名列表——那是浪费时间且容易错。

### 4.2 wire ProviderSet 冲突

若 `wire.go` 的 `ProviderSet` 仍冲突（一般是上游在列表里增删）：

- 上游新增的 Provider → 保留在 `ProviderSet`
- fork 新增的 Provider → 移到 `wire_fork.go` 的 `ForkProviderSet`（见 §2.2）

### 4.3 其他手写文件冲突

按业务语义正常 merge，取并集 / 按意图取舍。

---

## 5. 减少未来冲突的通用原则

1. **新增功能尽量放新文件**：新文件永不冲突；原地改上游共享文件才冲突。
2. **fork 定制集中、隔离**：能抽到 fork 专属文件 / set 的就抽出去（如 `ForkProviderSet`）。
3. **高频小步合并上游**：分叉越久攒得越多，冲突越大。建议定期（如每周）合并。
4. **生成代码靠重新生成、不靠手改**：见 §4.1。

---

## 6. 已知坑

### wire 生成报 `missing go.sum entry for ... google/subcommands`

`go generate ./cmd/server`（即 `make generate` 的第二步）可能报：

```
missing go.sum entry for module providing package github.com/google/subcommands
(imported by github.com/google/wire/cmd/wire)
```

这是 wire **代码生成工具**自身的依赖未进 `go.sum`（与应用运行无关），上游和 fork 都存在。修复：

```bash
cd backend
go get github.com/google/wire/cmd/wire@v0.7.0   # Go 会补齐 go.sum 条目
```

> 注：`go mod tidy` 可能再次裁掉这条仅工具用到的依赖。若希望长期固定，可引入 `tools.go`（build tag `//go:build tools`）显式 import 生成工具。
> 临时验证不想污染依赖文件时：本次重构已确认 wire 生成结果与原来逐字节相同，`wire_gen.go` 无需重新生成即有效。

---

**最后更新**：2026-05-28

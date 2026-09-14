# C311 Agent Evaluation 使用指南

本目录提供两个本地 runner，用同一个 OpenAI 兼容模型网关执行 C311 task：

- `run_swe_agent.py`：使用 SWE-agent。
- `run_openhands.py`：使用本地 Docker 中的 OpenHands Agent Canvas / Agent Server。

每次运行的流程相同：从 task 的 `B/customer-system` 复制全新候选项目，将
`R/requirement.md`、`B/constraints.md` 和 `B/customer-context.md` 写到候选项目的
`.benchmark-task/`，由 agent 实现需求，再执行该 task 原有的 `evaluation/run.sh`。
agent 不应读取 `ref-answer/`，runner 也不会复制 reference answer。

## 一次性准备

需要 Docker Desktop（或 Docker Engine）、Python 3.11+、`uv` 和一个 OpenAI 兼容
模型网关。先进入本目录并创建环境：

```bash
cd agent-evaluation
uv venv --python 3.11 .venv
uv pip install --python .venv/bin/python -r requirements.txt
uv pip install --python .venv/bin/python -e tools/swe-agent
```

克隆 agent 依赖到被 `.gitignore` 忽略的目录：

```bash
git clone --depth 1 https://github.com/SWE-agent/SWE-agent.git tools/swe-agent
```

创建私有配置，不要提交它：

```bash
cp .env.example .env
```

`.env` 需要三个字段：

```dotenv
OPENAI_API_KEY="..."
OPENAI_BASE_URL="https://your-gateway.example/v1"
MODEL="gpt-5.6"
```

可先确认模型通路；该命令不会显示密钥：

```bash
.venv/bin/python scripts/smoke_model.py "model gateway ready"
```

## 运行一个 SWE-agent task

```bash
.venv/bin/python run_swe_agent.py \
  /absolute/path/to/C311-FUNC-FND-01
```

默认单 task 的 SWE-agent 上限是 3 美元等值调用预算，可按需要调整：

```bash
.venv/bin/python run_swe_agent.py --cost-limit 5 \
  /absolute/path/to/C311-REF-SYS-02
```

## 运行一个 OpenHands task

```bash
.venv/bin/python run_openhands.py \
  /absolute/path/to/C311-REF-FND-01
```

首次执行会拉取并启动 `ghcr.io/openhands/agent-canvas:1.17.0`，只绑定
`127.0.0.1:18000`。可在浏览器打开 `http://127.0.0.1:18000/canvas` 查看本地
OpenHands 控制台。常用限制参数：

```bash
.venv/bin/python run_openhands.py --max-iterations 60 --timeout 3600 \
  /absolute/path/to/C311-REF-SYS-05
```

## 输出与结果解释

两个 runner 最后都会向标准输出打印一行 JSON，例如：

```json
{"agent":"swe-agent","task_id":"C311-FUNC-FND-01","status":"pass"}
```

完整日志、候选源码和 agent 轨迹默认保存在下列本机忽略目录：

```text
logs/<agent>/<task-run>/
workspaces/<agent>/<task-run>/candidate/
```

`status: pass` 表示 task 自带 evaluator 返回 0；`fail` 表示 evaluator 未通过，
不是 runner 自行判断。评测前应确认每一条 evaluator 断言都能在 R 或 B 的客户约束
中找到对应要求。若 evaluator 需要某个 HTTP 字段、状态码、持久化结果或授权结果，
但 R 和 B 均未声明，失败不能解释为 agent 没有完成已给需求。

## 安全边界

- `.env`、虚拟环境、Docker/ OpenHands 状态、日志、候选项目和第三方 agent 源码均被忽略。
- OpenHands 仅监听回环地址，不公开网络端口。
- 不要把 `ref-answer/` 作为 agent 工作目录或 prompt 的一部分。
- `reference-slice.sha256` 用于维护者验证 reference provenance，不应要求候选系统逐文件复制参考源码。

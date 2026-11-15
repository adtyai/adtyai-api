# constant 包 (`/constant`)

该目录仅用于放置全局可复用的**常量定义**，不包含任何业务逻辑或依赖关系。

## 当前文件

| 文件                   | 说明                                                                                                                                    |
|----------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `api_type.go`        | 上游 API 类型（供应商协议）枚举，如 `APITypeOpenAI`、`APITypeAnthropic`、`APITypeGemini`。                                                                 |
| `azure.go`           | 定义与 Azure 相关的全局常量，如 `AzureNoRemoveDotTime`（控制删除 `.` 的截止时间）。                                                                                |
| `cache_key.go`       | 缓存键格式字符串及 Token 相关字段常量，统一缓存命名规则。                                                                                                      |
| `channel.go`         | 渠道类型（`ChannelType*`）与渠道状态常量，如 OpenAI、Azure、Claude、Gemini 等。                                                                             |
| `context_key.go`     | 定义 `ContextKey` 类型以及在整个项目中使用的上下文键常量（请求时间、Token/Channel/User 相关信息等）。                                                                   |
| `endpoint_type.go`   | 中转端点类型 `EndpointType*`，是 `relaykit/types` 中同名类型的别名；另有 DeepSeek 搜索/抓取用的模型名。                                                                |
| `env.go`             | 环境配置相关的全局变量，在启动阶段根据配置文件或环境变量注入。                                                                                                      |
| `finish_reason.go`   | OpenAI/GPT 请求返回的 `finish_reason` 取值，是 `relaykit/types` 中同名变量的别名。                                                                          |
| `midjourney.go`      | Midjourney 相关错误码及动作(Action)常量与模型到动作的映射表。                                                                                               |
| `multi_key_mode.go`  | 多渠道密钥的选择模式，如 `random`（随机）、`polling`（轮询）。                                                                                             |
| `setup.go`           | 标识项目是否已完成初始化安装 (`Setup` 布尔值)。                                                                                                        |
| `task.go`            | 各种任务(Task)平台、动作常量及模型与动作映射表，如 Suno、Midjourney 等。                                                                                     |
| `waffo_pay_method.go` | Waffo 支付方式的展示名、图标与 API 参数映射，含默认支付方式列表 `DefaultWaffoPayMethods`。                                                                         |

`README.md` 本身和 `channel_test.go`、`task_test.go` 是文档与测试文件，不是常量定义。原先的 `channel_setting.go`、`user_setting.go` 已随 DTO 一起迁到 `relaykit/dto/channel_settings.go`、`relaykit/dto/user_settings.go`，本目录不再有这两个文件。

## 使用约定

1. `constant` 包**只能被其他包引用**（import），**禁止在此包中引用项目内带业务依赖的自定义包**。如确有需要，仅允许引用 **Go 标准库**，和无业务依赖的基础包（目前是 `relaykit/types`，见 `endpoint_type.go`、`finish_reason.go`）。
2. 不允许在此目录内编写任何与业务流程、数据库操作、第三方服务调用等相关的逻辑代码。
3. 新增类型时，请保持命名语义清晰，并在本 README 的 **当前文件** 表格中补充说明，确保团队成员能够快速了解其用途。

> ⚠️ 违反以上约定将导致包之间产生不必要的耦合，影响代码可维护性与可测试性。请在提交代码前自行检查。
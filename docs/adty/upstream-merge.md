# 合并上游

上游 [QuantumNous/new-api](https://github.com/QuantumNous/new-api) 每出一个 release，照下面的流程合并一次（实施方案第七节）。合并脚本 `scripts/adty/merge-upstream.sh` 按删除清单 [deleted-paths.txt](deleted-paths.txt) 处理冲突，不让删掉的文件跟着上游回来；剩下的冲突由人对照 [upstream-touchpoints.md](upstream-touchpoints.md) 解决。

## 准备

远程仓库 `upstream` 指向上游，并且禁止推送。新克隆的仓库先加上：

```bash
git remote add upstream https://github.com/QuantumNous/new-api.git
git remote set-url --push upstream DISABLE
git config remote.upstream.tagopt --no-tags
```

`tagopt --no-tags` 让平常的 `git fetch upstream` 也不把上游标签带进 `refs/tags/`：我们的发布标签也叫 `v*`，同名时 fetch 会报 would clobber；上游标签进了 `refs/tags/`，`git push --tags` 还会把它们推到我们的仓库。合并脚本自己把上游标签取到 `refs/upstream-tags/`。不要 `git push --tags`，发布时只推要发的那一个标签。

## 流程

1. 备份生产数据库和 /data。
2. 在 Windows 上用 Git Bash 跑合并脚本，用的是 Windows 的 git。在仓库根目录的 PowerShell 里：

   ```powershell
   & "C:\Program Files\Git\bin\bash.exe" scripts/adty/merge-upstream.sh <新标签>
   ```

   `C:\Program Files\Git` 是 Git for Windows 的默认安装位置；在 Git Bash 窗口里直接跑 `bash scripts/adty/merge-upstream.sh <新标签>`。参数也可以是提交号。

   脚本先 `git fetch --no-tags upstream`，把上游的分支取到 `refs/remotes/upstream/`、标签取到 `refs/upstream-tags/`（不进 `refs/tags/`：我们的发布标签也叫 `v*`，和上游同名时取进 `refs/tags/` 会因为「would clobber existing tag」失败）；再从 main 新建分支 `merge/<新标签>`，`git -c core.quotepath=false merge --no-ff --no-commit refs/upstream-tags/<新标签>`（参数不是上游标签时按本地的分支或提交合并），然后按删除清单处理「我们删了、上游改了」（DU）的冲突和上游在已删目录里新增的文件，见下面的「脚本怎么处理冲突」。合并不提交，最后列出剩下的冲突。
3. 解决剩下的冲突：
   - 对照 upstream-touchpoints.md（我们改过的每个上游文件、改了什么、为什么）和 [conventions.md](conventions.md) 约定 B 列的主要挂载点。
   - 上游在共享文件里新加了订阅、兑换码、上游邀请奖励的代码，一并删掉，补进 [deleted-features.md](deleted-features.md)；整个删掉的文件和目录同时补进 deleted-paths.txt。
   - 上游新出现的 New API 字样，按任务 8.9 的表处理。
   - `web/src/routeTree.gen.ts` 有冲突不手工合（约定 B）：先 `git checkout --theirs -- web/src/routeTree.gen.ts` 去掉冲突标记，第 4 步重新生成。
   - `web/bun.lock` 有冲突也不手工合：先 `git checkout --theirs -- web/bun.lock`，合好 `web/package.json`，再在副本里 `bun install` 重新生成，用 adty-pull.sh 拉回来。
   - 处理完一个冲突就 `git add` 或 `git rm` 一个。
4. 重新生成和刷新。下面的命令在 WSL 的测试副本 `~/adtyai/mirror/main` 里跑：写进任务脚本，经 wrun.sh 运行，它先把 Windows 仓库的工作区同步过去（约定 H）。
   - 在 `web` 里先 `bun install --frozen-lockfile`（上游常改依赖），再 `bun run build` 重新生成 `web/src/routeTree.gen.ts`，再 `bun run i18n:sync`。然后在副本根目录用 `bash ~/adtyai/bin/adty-pull.sh web/src/routeTree.gen.ts web/src/i18n/locales/*.json` 把 routeTree.gen.ts 和语言包拉回 Windows 仓库。
   - 刷新 `web/scripts/adty/upstream-en-keys.json`（任务 9.2 起有）：换成新标签的 `web/src/i18n/locales/en.json` 的键。
   - 上游新加的后端键补进 fr、ja、ru、vi 四个上游 yaml（任务 9.1 起有）；`TestLocalesRenderEveryKey` 会列出缺哪些。
   - 刷新 check.sh 用的 lint 和版权头基线清单（任务 0.3）：在副本里 `node scripts/adty/ratchet.mjs --update`，用 adty-pull.sh 拉回 `scripts/adty/baseline-lint.txt` 和 `scripts/adty/baseline-copyright.txt`，再用 `git diff scripts/adty/` 确认新增的只有上游文件（见 [baseline.md](baseline.md) 的「基线清单」）。
5. 在副本里跑 `bash scripts/adty/check.sh`。重点看：
   - 删除守卫测试（任务 1.6）：`TestAdtyDeletedRoutesAbsent`、`TestAdtyDeletedSchemaAbsent`、`TestAdtyDeletedOptionKeysAbsent`，以及前端的 `web/src/features/adty-guards/__tests__/deleted-features.test.ts`。
   - 品牌守卫测试（任务 8.9）：`common/adty_brand_guard_test.go`、`web/src/features/adty-guards/__tests__/brand.test.ts`。任务 8.9 做完才有。
   - 后台权限（任务 2.6）：`TestMainEnablesAdtyAdminPermissions`、`TestEveryUpstreamAdminScopeClassified`、`TestAdminRouteInventory`。上游新加的管理接口按 2.6 归类，更新 `middleware/testdata/adty_admin_routes.txt`。任务 2.6 做完才有。

   都通过后 `git commit` 完成合并，提交身份按约定 H 写在命令里。
6. 推送分支：`git push origin merge/<新标签>`，等 CI 跑完。
7. 在测试环境部署这个分支的镜像，做客户端回归：用 Claude Code、Codex CLI、Cherry Studio 等真实客户端，测流式输出、工具调用、提示缓存命中、思考模式、长上下文和图片输入，都通过再上线（设计稿 §12 第 2 条）。镜像是 `ghcr.io/adtyai/adtyai-api:merge-<新标签>`：`.github/workflows/adty-ghcr.yml` 在 push 到 `merge/*` 分支时也推送，只打这一个标签，不打 sha 和 latest；同一个分支连续推送时只留最新的一次运行。
8. 合并到 main 并推送。CI 推出 `main` 和 `sha-<7 位提交号>` 两个镜像标签，生产环境换成新的 sha 标签；有问题换回上一个 sha 标签。

放弃这次合并：`git merge --abort`，再 `git switch main` 和 `git branch -D merge/<新标签>`。

## 脚本怎么处理冲突

脚本读 `git status --porcelain -z`，路径里的空格、中文原样处理。「在清单里」指路径等于清单里的文件，或者在清单里的目录下（格式见 deleted-paths.txt 开头的注释）。

| 状态 | 意思 | 脚本怎么做 |
| --- | --- | --- |
| `DU`，路径在清单里 | 我们删了、上游改了 | `git rm` |
| `DU`，路径不在清单里 | 我们删了、上游改了。上游给我们删掉的文件改了名也是这样，列的是新路径 | 留给人工：是删掉的功能就 `git rm`，再把新路径补进清单 |
| `A `（冲突时是 `UA`），路径在清单里 | 上游新增的文件。落在已删目录里的不会冲突，不处理就会悄悄带回来 | 列出来并 `git rm` |
| `UD` | 上游删了、我们改了 | 留给人工，不按清单处理 |
| 其他（`UU`、`AA`、`AU`、`DD`，以及路径不在清单里的 `UA`） | 两边都改了、两边都新增了等 | 留给人工 |

- 只删 HEAD（合并前的 main）里没有的路径，也就是只删上游带进来的东西。`AU` 是「我们新增」，只有我们这边的版本，所以不自动删。
- 上游把别处的文件改名挪进已删目录，也算新增，会被删掉。我们还在用这个文件的话，第 5 步的检查会报错。

## 脚本什么时候不跑

下面这些情况，脚本不建分支、不动工作区就退出（参数不对是退出码 2，其余是 1）。只有最后一条的后两种情况发生在 fetch 之后，这时 upstream 的远程分支和标签已经更新：

- 参数不是一个，或者以 `-` 开头。
- 在 WSL 里经 /mnt 打开 Windows 仓库（那样用的是 WSL 的 git），或者脚本不在仓库的 `scripts/adty/` 下。
- 没有远程仓库 `upstream`。
- 当前分支不是 main；有做到一半的 merge、cherry-pick、revert、rebase 或 am。
- 工作区不干净，没跟踪的文件也算。
- 删除清单有写法不对的行：开头带 `/`、有反斜杠、前后有空格、有 `//`、`.` 或 `..`。
- `merge/<参数>` 不能当分支名，或者已经有这个分支。
- `git fetch upstream` 失败；fetch 以后找不到这个标签或提交；它已经在 main 里了。

分支建好以后 `git merge` 没能开始（比如两边没有共同的历史），脚本以退出码 1 结束，分支留着，内容和 main 一样，并提示怎么删掉。合并开始以后，脚本处理完就以退出码 0 结束，不管还剩多少冲突；只有 `git rm` 失败时是 1。

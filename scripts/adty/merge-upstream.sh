#!/usr/bin/env bash
# 合并上游的一个版本（实施方案任务 1.7），整个流程见 docs/adty/upstream-merge.md。
# 在 Windows 上用 Git Bash 运行，用的是 Windows 的 git。在仓库根目录的 PowerShell 里：
#   & "C:\Program Files\Git\bin\bash.exe" scripts/adty/merge-upstream.sh <上游标签或提交>
# 做的事：git fetch --no-tags upstream（上游标签取到 refs/upstream-tags/，不和我们的 v* 标签冲突）→
# 从 main 新建分支 merge/<参数> → git -c core.quotepath=false merge --no-ff --no-commit <参数>
# （参数是上游标签时合并 refs/upstream-tags/<参数>）→ 读 git status --porcelain -z，
# 按删除清单 docs/adty/deleted-paths.txt 处理，不让删掉的文件跟着上游回来：
#   - DU（我们删了、上游改了）且路径在清单里：git rm；
#   - 上游新增的文件（A，冲突时是 UA）路径在清单里（通常落在清单的目录下）：列出来并 git rm；
#   - 只删 HEAD 里没有的路径，也就是只删上游带进来的东西；
#   - 其余的留给人工：路径不在清单里的 DU（上游给我们删掉的文件改了名，列的是新路径）、
#     UD（上游删了、我们改了，不能按清单处理）和别的冲突。
# 合并不提交，最后列出剩下的冲突和下一步。
# 不在 main 上、工作区不干净、分支已存在、找不到标签或提交这类情况，不建分支、不动工作区就退出（退出码 1）。
set -euo pipefail

list_file=docs/adty/deleted-paths.txt

# 删除清单：deleted_files 按整条路径匹配，deleted_dirs 以 / 结尾、按前缀匹配
deleted_files=()
deleted_dirs=()

# load_status 读出的 git status：第 i 条的状态是 st_xy[i]，路径是 st_path[i]
st_xy=()
st_path=()
status_file=''

usage() {
  echo "用法：bash scripts/adty/merge-upstream.sh <上游标签或提交>" >&2
  echo "在 Windows 上用 Git Bash 运行，流程见 docs/adty/upstream-merge.md" >&2
  exit 2
}

# 每个参数打印一行原因，然后以退出码 1 结束
die() {
  local line
  for line in "$@"; do
    printf 'merge-upstream.sh: %s\n' "$line" >&2
  done
  exit 1
}

step() {
  printf '\n==> %s\n' "$*"
}

# 第一个参数是标题，其余是路径
print_paths() {
  local title=$1
  shift
  echo "$title"
  if [ "$#" -eq 0 ]; then
    echo "  （没有）"
  else
    printf '  %s\n' "$@"
  fi
}

# 在仓库根目录，用的是 Windows 的 git，有远程仓库 upstream
check_repo() {
  local repo_root cdup
  repo_root=$(pwd -P)
  # WSL 里经 /mnt 打开 Windows 仓库，用的是 WSL 的 git；约定 H：WSL 里的 git 只做只读操作
  case "$repo_root" in
    /mnt | /mnt/*)
      die "仓库目录 $repo_root 在 /mnt 下，这是在 WSL 里，用的是 WSL 的 git。" \
        "在 Windows 上用 Git Bash 运行，见 docs/adty/upstream-merge.md。"
      ;;
  esac
  cdup=$(git rev-parse --show-cdup) || die "$repo_root 不在 git 仓库里。"
  [ -z "$cdup" ] || die "$repo_root 不是仓库的根目录，脚本要放在仓库的 scripts/adty/ 下。"
  git remote get-url upstream > /dev/null 2>&1 \
    || die "没有远程仓库 upstream。先加上：git remote add upstream https://github.com/QuantumNous/new-api.git"
}

# 在 main 上，没有做到一半的操作，工作区干净（没跟踪的文件也不能有）
check_worktree() {
  local current name dirty
  current=$(git symbolic-ref --quiet --short HEAD) || current=''
  [ "$current" = main ] \
    || die "当前不在 main 上（${current:-HEAD 没有指向分支}）。合并分支从 main 建，先 git switch main。"
  for name in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD; do
    if git rev-parse --quiet --verify "$name" > /dev/null; then
      die "有做到一半的 merge、cherry-pick 或 revert（有 $name），先做完或者放弃。"
    fi
  done
  for name in rebase-merge rebase-apply; do
    if [ -e "$(git rev-parse --git-path "$name")" ]; then
      die "有做到一半的 rebase 或 am（有 $name），先做完或者放弃。"
    fi
  done
  dirty=$(git -c core.quotepath=false status --porcelain --untracked-files=normal)
  if [ -n "$dirty" ]; then
    printf '%s\n' "$dirty" >&2
    die "工作区不干净（上面是没提交的改动和没跟踪的文件），先提交或者移走。"
  fi
}

# 读删除清单，格式见它开头的注释：# 开头的行和空行跳过；目录以 / 结尾
load_deleted_list() {
  local line lineno=0
  [ -f "$list_file" ] || die "找不到删除清单 $list_file。"
  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$((lineno + 1))
    line=${line%$'\r'}
    case "$line" in
      '' | '#'*) continue ;;
    esac
    case "$line" in
      # 写法不对的行什么也匹配不上，保护会悄悄失效，所以直接拒绝
      /* | *\\* | [[:space:]]* | *[[:space:]] | *//* | . | .. | ./* | ../* | */. | */.. | */./* | */../*)
        die "$list_file 第 $lineno 行写法不对：$line" \
          "路径从仓库根目录写起，开头不加 /，用 / 分隔，前后不带空格，不写 . 和 ..。"
        ;;
      */) deleted_dirs+=("$line") ;;
      *) deleted_files+=("$line") ;;
    esac
  done < "$list_file"
}

# merge/<参数> 能当分支名，而且还没有这个分支
check_branch() {
  local branch=$1
  git check-ref-format "refs/heads/$branch" \
    || die "$branch 不能当分支名。参数写上游的标签名或提交号。"
  if git show-ref --verify --quiet "refs/heads/$branch"; then
    die "分支 $branch 已经存在。确认里面没有要留的东西后 git branch -D $branch，再重新运行。"
  fi
}

# 路径在删除清单里：等于清单里的文件，或者在清单里的目录下
is_listed() {
  local path=$1 entry
  for entry in "${deleted_files[@]}"; do
    if [ "$path" = "$entry" ]; then
      return 0
    fi
  done
  for entry in "${deleted_dirs[@]}"; do
    if [[ $path == "$entry"* ]]; then
      return 0
    fi
  done
  return 1
}

# HEAD（合并前的 main）里有这个文件
in_head() {
  git cat-file -e "HEAD:$1" 2> /dev/null
}

# 读 git status --porcelain -z 到 st_xy、st_path。每条是「XY 路径」，以 NUL 结尾；路径不转义，
# 空格、中文原样保留。改名和复制的后面还跟一条原路径，读掉。--no-renames 让上游改名的文件
# 按「删掉旧路径、新增新路径」列出，新路径落在清单里也能认出来
load_status() {
  local rec
  st_xy=()
  st_path=()
  git -c core.quotepath=false status --porcelain -z --no-renames --untracked-files=no > "$status_file"
  while IFS= read -r -d '' rec; do
    st_xy+=("${rec:0:2}")
    st_path+=("${rec:3}")
    case "${rec:0:2}" in
      *R* | *C*) IFS= read -r -d '' rec || true ;;
    esac
  done < "$status_file"
}

# 删掉上游带回来的、删除清单里的文件：路径在清单里，HEAD 里没有，状态是 DU 或者新增
remove_deleted_paths() {
  local i xy path
  local -a du_paths=() added_paths=() paths=()
  step "按删除清单处理"
  echo "$list_file：${#deleted_files[@]} 个文件、${#deleted_dirs[@]} 个目录"
  load_status
  for i in "${!st_xy[@]}"; do
    xy=${st_xy[$i]}
    path=${st_path[$i]}
    if ! is_listed "$path" || in_head "$path"; then
      continue
    fi
    case "$xy" in
      DU) du_paths+=("$path") ;;
      # A 是合并时干净地新增，UA 是冲突里的「上游新增」（只有上游的版本）。
      # AU 是「我们新增」（只有我们的版本），不是上游带来的，留给人工
      'A ' | AM | UA) added_paths+=("$path") ;;
    esac
  done
  paths=("${du_paths[@]}" "${added_paths[@]}")
  if [ "${#paths[@]}" -gt 0 ]; then
    # 路径经标准输入以 NUL 分隔交给 git，不受命令行长度限制；--literal-pathspecs 让 * ? [ 按字面匹配。
    # -f：新增的文件暂存区里有、HEAD 里没有，不加 -f 的话 git rm 会拒绝。这些路径 HEAD 里都没有，
    # 工作区里是合并刚写出来的上游版本，删掉不会丢我们的东西
    printf '%s\0' "${paths[@]}" \
      | git --literal-pathspecs rm -q -f --pathspec-from-file=- --pathspec-file-nul \
      || die "git rm 失败，见上面 git 的输出。合并还在进行：可以手工处理，或者 git merge --abort 放弃。"
  fi
  print_paths "我们删了、上游改了（DU），路径在清单里，已经 git rm：" "${du_paths[@]}"
  print_paths "上游新增、路径在清单里（多半落在删掉的目录下），已经 git rm：" "${added_paths[@]}"
}

# 列出剩下的冲突（git status 里没合并的路径）和下一步
report_conflicts() {
  local branch=$1 i xy path total
  local -a du=() ud=() other=()
  load_status
  for i in "${!st_xy[@]}"; do
    xy=${st_xy[$i]}
    path=${st_path[$i]}
    case "$xy" in
      DU) du+=("$path") ;;
      UD) ud+=("$path") ;;
      DD | AU | UA | AA | UU) other+=("$xy $path") ;;
    esac
  done
  total=$((${#du[@]} + ${#ud[@]} + ${#other[@]}))
  if [ "$total" -eq 0 ]; then
    step "剩下的冲突：没有"
  else
    step "剩下的冲突（$total 个），要人工处理"
  fi
  if [ "${#du[@]}" -gt 0 ]; then
    echo "DU：我们删了、上游改了，路径不在删除清单里。多半是上游给我们删掉的文件改了名，列的是新路径。"
    echo "    是删掉的功能就 git rm，再把新路径补进 $list_file；要留就 git add。"
    printf '  %s\n' "${du[@]}"
  fi
  if [ "${#ud[@]}" -gt 0 ]; then
    echo "UD：上游删了、我们改了，不按清单处理。要留就 git add，不要就 git rm。"
    printf '  %s\n' "${ud[@]}"
  fi
  if [ "${#other[@]}" -gt 0 ]; then
    echo "其他冲突（UU 两边都改了，AA 两边都新增了；AU、UA、DD 多半和改名有关）："
    printf '  %s\n' "${other[@]}"
  fi
  step "下一步（详见 docs/adty/upstream-merge.md）"
  cat << EOF
1. 剩下的冲突对照 docs/adty/upstream-touchpoints.md（我们改过哪些上游文件、为什么改）解决，处理完一个就 git add 或 git rm 一个。
2. web/src/routeTree.gen.ts 不手工合：在 WSL 测试副本里 bun run build 重新生成，用 adty-pull.sh 拉回来再 git add。
3. 跑守卫测试：在 WSL 测试副本里跑 bash scripts/adty/check.sh。删掉的功能跟着上游回来时，删除守卫测试（任务 1.6）会失败。
4. 合并还没提交，检查都通过后再 git commit。放弃这次合并：git merge --abort，再 git switch main，git branch -D $branch。
EOF
}

main() {
  local ref branch commit merge_ref rc=0
  local upstream_tags=refs/upstream-tags/
  [ "$#" -eq 1 ] || usage
  ref=$1
  case "$ref" in
    '' | -*) usage ;;
  esac
  branch="merge/$ref"

  # 脚本在 scripts/adty/ 下，仓库根目录在上两级
  cd "$(dirname "${BASH_SOURCE[0]}")/../.."
  check_repo
  check_worktree
  load_deleted_list
  check_branch "$branch"

  # 上游的标签取到 refs/upstream-tags/ 下，不进 refs/tags/：我们的发布标签也叫 v*，
  # 和上游同名但指向不同提交时，取进 refs/tags/ 会因为「would clobber existing tag」整个失败。
  # --no-tags 关掉标签的自动跟随，理由相同
  step "git fetch --no-tags upstream（标签取到 $upstream_tags）"
  git fetch --no-tags upstream "+refs/heads/*:refs/remotes/upstream/*" "+refs/tags/*:$upstream_tags*" \
    || die "git fetch upstream 失败，见上面 git 的输出。"
  merge_ref=$upstream_tags$ref
  if ! git rev-parse --quiet --verify "$merge_ref^{commit}" > /dev/null; then
    merge_ref=$ref
  fi
  commit=$(git rev-parse --quiet --verify "$merge_ref^{commit}") \
    || die "fetch 以后还是找不到 $ref：它不是 upstream 的标签，也不是本地的分支或提交。"
  if git merge-base --is-ancestor "$commit" HEAD; then
    die "$ref（$commit）已经在 main 里了，没有要合并的东西。"
  fi

  step "从 main（$(git rev-parse --short HEAD)）新建分支 $branch"
  git switch -c "$branch"

  step "git merge --no-ff --no-commit $merge_ref（$commit）"
  git -c core.quotepath=false merge --no-ff --no-commit "$merge_ref" || rc=$?
  if ! git rev-parse --quiet --verify MERGE_HEAD > /dev/null; then
    die "git merge 没有开始合并（退出码 $rc），见上面 git 的输出。" \
      "现在在分支 $branch 上，内容和 main 一样。不要了就 git switch main，再 git branch -D $branch。"
  fi

  status_file=$(mktemp)
  trap 'rm -f "$status_file"' EXIT
  remove_deleted_paths
  report_conflicts "$branch"
}

main "$@"
